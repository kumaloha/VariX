package contentstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

var defaultSubjectExperienceHorizons = []string{"1w", "1m", "1q", "1y", "2y", "5y"}

func (s *SQLiteStore) GetSubjectExperienceMemory(ctx context.Context, userID, subject string, horizons []string, now time.Time, refresh bool) (memory.SubjectExperienceMemory, error) {
	return s.getSubjectExperienceMemoryWithHorizonInputs(ctx, userID, subject, horizons, now, refresh, nil)
}

func (s *SQLiteStore) getSubjectExperienceMemoryWithHorizonInputs(ctx context.Context, userID, subject string, horizons []string, now time.Time, refresh bool, preloaded map[string]memory.SubjectHorizonMemory) (memory.SubjectExperienceMemory, error) {
	userID, err := normalizeRequiredUserID(userID)
	if err != nil {
		return memory.SubjectExperienceMemory{}, err
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return memory.SubjectExperienceMemory{}, fmt.Errorf("subject is required")
	}
	horizons, err = normalizeSubjectExperienceHorizons(horizons)
	if err != nil {
		return memory.SubjectExperienceMemory{}, err
	}
	now = normalizeNow(now)
	canonicalSubject, err := s.resolveCanonicalListSubject(ctx, subject)
	if err != nil {
		return memory.SubjectExperienceMemory{}, err
	}
	horizonMemories := make([]memory.SubjectHorizonMemory, 0, len(horizons))
	for _, horizon := range horizons {
		if item, ok := preloaded[strings.TrimSpace(horizon)]; ok {
			horizonMemories = append(horizonMemories, item)
			continue
		}
		item, err := s.GetSubjectHorizonMemory(ctx, userID, subject, horizon, now, refresh)
		if err != nil {
			return memory.SubjectExperienceMemory{}, err
		}
		horizonMemories = append(horizonMemories, item)
	}
	inputHash := subjectExperienceInputHash(horizonMemories)
	horizonSet := strings.Join(horizons, ",")
	if !refresh {
		cached, ok, err := s.getCachedSubjectExperienceMemory(ctx, userID, canonicalSubject, horizonSet, inputHash)
		if err != nil {
			return memory.SubjectExperienceMemory{}, err
		}
		if ok {
			return cached, nil
		}
	}
	out := buildSubjectExperienceMemory(userID, subject, canonicalSubject, horizons, horizonMemories, inputHash, now)
	if err := s.upsertSubjectExperienceMemory(ctx, horizonSet, out); err != nil {
		return memory.SubjectExperienceMemory{}, err
	}
	return out, nil
}

func normalizeSubjectExperienceHorizons(horizons []string) ([]string, error) {
	if len(horizons) == 0 {
		return append([]string(nil), defaultSubjectExperienceHorizons...), nil
	}
	out := make([]string, 0, len(horizons))
	seen := map[string]struct{}{}
	for _, horizon := range horizons {
		horizon = strings.TrimSpace(horizon)
		if horizon == "" {
			continue
		}
		spec, err := subjectHorizonSpecFor(horizon)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[spec.Horizon]; ok {
			continue
		}
		seen[spec.Horizon] = struct{}{}
		out = append(out, spec.Horizon)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one horizon is required")
	}
	return out, nil
}

func buildSubjectExperienceMemory(userID, subject, canonicalSubject string, horizons []string, horizonMemories []memory.SubjectHorizonMemory, inputHash string, now time.Time) memory.SubjectExperienceMemory {
	generatedAt := now.UTC().Format(time.RFC3339)
	out := memory.SubjectExperienceMemory{
		UserID:           userID,
		Subject:          subject,
		CanonicalSubject: canonicalSubject,
		Horizons:         append([]string(nil), horizons...),
		GeneratedAt:      generatedAt,
		CacheStatus:      "refreshed",
		InputHash:        inputHash,
	}
	out.HorizonSummaries = subjectExperienceHorizonSummaries(horizonMemories)
	out.EvidenceSourceRefs = subjectExperienceEvidenceRefs(horizonMemories)
	out.Lessons = deriveSubjectExperienceLessons(firstTrimmed(canonicalSubject, subject), horizonMemories)
	out.AttributionSummary = buildSubjectAttributionSummary(horizonMemories)
	out.LessonCount = len(out.Lessons)
	out.Abstraction = summarizeSubjectExperience(out)
	return out
}

func subjectExperienceHorizonSummaries(horizonMemories []memory.SubjectHorizonMemory) []memory.SubjectExperienceHorizon {
	out := make([]memory.SubjectExperienceHorizon, 0, len(horizonMemories))
	for _, item := range horizonMemories {
		topDrivers := make([]string, 0, 3)
		for i, driver := range item.DriverClusters {
			if i >= 3 {
				break
			}
			topDrivers = append(topDrivers, driver.Subject)
		}
		out = append(out, memory.SubjectExperienceHorizon{
			Horizon:         item.Horizon,
			SampleCount:     item.SampleCount,
			TrendDirection:  item.TrendDirection,
			VolatilityState: item.VolatilityState,
			TopDrivers:      topDrivers,
		})
	}
	return out
}

func subjectExperienceEvidenceRefs(horizonMemories []memory.SubjectHorizonMemory) []string {
	refs := make([]string, 0)
	for _, item := range horizonMemories {
		refs = append(refs, item.EvidenceSourceRefs...)
	}
	return uniqueStrings(refs)
}

type subjectDriverExperience struct {
	subject    string
	count      int
	horizons   []string
	changes    []string
	paths      []string
	sourceRefs []string
}

type subjectAttributionFactorState struct {
	subject  string
	count    int
	refs     []string
	sources  []string
	horizons []string
}

type subjectAttributionChangeState struct {
	when       string
	change     string
	externalID string
	sourceBase string
	factors    []string
}

type subjectAttributionRelationState struct {
	factors []string
	sources []string
}

func buildSubjectAttributionSummary(horizonMemories []memory.SubjectHorizonMemory) memory.SubjectAttributionSummary {
	factors := map[string]*subjectAttributionFactorState{}
	changes := map[string]*subjectAttributionChangeState{}
	sourceFactors := map[string][]string{}
	relations := map[string]*subjectAttributionRelationState{}
	for _, horizon := range horizonMemories {
		for _, driver := range horizon.DriverClusters {
			subject := strings.TrimSpace(driver.Subject)
			if subject == "" {
				continue
			}
			state := factors[subject]
			if state == nil {
				state = &subjectAttributionFactorState{subject: subject}
				factors[subject] = state
			}
			state.refs = uniqueStrings(append(state.refs, driver.SourceRefs...))
			state.count = len(state.refs)
			state.horizons = orderedUniqueStrings(append(state.horizons, horizon.Horizon))
			for _, ref := range driver.SourceRefs {
				source := sourceRefBase(ref)
				if source == "" {
					continue
				}
				state.sources = orderedUniqueStrings(append(state.sources, source))
				sourceFactors[source] = orderedUniqueStrings(append(sourceFactors[source], subject))
			}
		}
		for _, change := range horizon.KeyChanges {
			sourceBase := change.SourcePlatform + ":" + change.SourceExternalID
			key := sourceBase + "#" + change.NodeID
			if _, ok := changes[key]; ok {
				continue
			}
			changes[key] = &subjectAttributionChangeState{
				when:       change.When,
				change:     change.ChangeText,
				externalID: change.SourceExternalID,
				sourceBase: sourceBase,
			}
		}
	}
	for _, change := range changes {
		change.factors = rankSubjectFactors(sourceFactors[change.sourceBase], factors)
		if len(change.factors) > 1 {
			keyFactors := append([]string(nil), change.factors...)
			sort.Strings(keyFactors)
			key := strings.Join(keyFactors, "\x00")
			state := relations[key]
			if state == nil {
				state = &subjectAttributionRelationState{factors: keyFactors}
				relations[key] = state
			}
			state.sources = orderedUniqueStrings(append(state.sources, change.sourceBase))
		}
	}
	changeList := make([]memory.SubjectChangeAttribution, 0, len(changes))
	for _, change := range changes {
		changeList = append(changeList, memory.SubjectChangeAttribution{
			When:             change.when,
			ChangeText:       change.change,
			Factors:          change.factors,
			SourceExternalID: change.externalID,
		})
	}
	sort.SliceStable(changeList, func(i, j int) bool {
		if changeList[i].When != changeList[j].When {
			return changeList[i].When < changeList[j].When
		}
		return changeList[i].SourceExternalID < changeList[j].SourceExternalID
	})
	primary := subjectAttributionPrimaryFactor(factors)
	relationList := make([]memory.SubjectFactorRelationship, 0, len(relations))
	for _, relation := range relations {
		relationList = append(relationList, memory.SubjectFactorRelationship{
			Factors:     append([]string(nil), relation.factors...),
			Relation:    subjectAttributionRelationLabel(relation.factors, primary.Subject),
			SourceCount: len(relation.sources),
			SourceRefs:  relation.sources,
		})
	}
	sort.SliceStable(relationList, func(i, j int) bool {
		if relationList[i].SourceCount != relationList[j].SourceCount {
			return relationList[i].SourceCount > relationList[j].SourceCount
		}
		leftKey := strings.Join(relationList[i].Factors, "\x00")
		rightKey := strings.Join(relationList[j].Factors, "\x00")
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		return relationList[i].Relation < relationList[j].Relation
	})
	if len(relationList) > 6 {
		relationList = relationList[:6]
	}
	return memory.SubjectAttributionSummary{
		ChangeCount:        len(changeList),
		FactorCount:        len(factors),
		PrimaryFactor:      primary,
		ChangeAttributions: changeList,
		FactorRelations:    relationList,
	}
}

func subjectAttributionRelationLabel(factors []string, primary string) string {
	primary = strings.TrimSpace(primary)
	if primary == "" || !containsOrderedString(factors, primary) {
		return "共同归因同一变化；当前按并列因素处理"
	}
	others := make([]string, 0, len(factors))
	for _, factor := range factors {
		if factor != primary {
			others = append(others, factor)
		}
	}
	return fmt.Sprintf("共同归因同一变化；%s是当前主导因素，%s是伴随因素", primary, strings.Join(others, "、"))
}

func subjectAttributionPrimaryFactor(factors map[string]*subjectAttributionFactorState) memory.SubjectPrimaryFactor {
	list := make([]*subjectAttributionFactorState, 0, len(factors))
	for _, factor := range factors {
		list = append(list, factor)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].count != list[j].count {
			return list[i].count > list[j].count
		}
		if len(list[i].sources) != len(list[j].sources) {
			return len(list[i].sources) > len(list[j].sources)
		}
		return list[i].subject < list[j].subject
	})
	if len(list) == 0 {
		return memory.SubjectPrimaryFactor{}
	}
	top := list[0]
	return memory.SubjectPrimaryFactor{
		Subject:     top.subject,
		Reason:      fmt.Sprintf("在当前样本中出现次数最多，覆盖 %d 个来源、%d 个时间尺度。", len(top.sources), len(top.horizons)),
		Support:     top.count,
		SourceCount: len(top.sources),
		Horizons:    append([]string(nil), top.horizons...),
	}
}

func rankSubjectFactors(items []string, factors map[string]*subjectAttributionFactorState) []string {
	out := orderedUniqueStrings(items)
	sort.SliceStable(out, func(i, j int) bool {
		left := factors[out[i]]
		right := factors[out[j]]
		leftCount, rightCount := 0, 0
		if left != nil {
			leftCount = left.count
		}
		if right != nil {
			rightCount = right.count
		}
		if leftCount != rightCount {
			return leftCount > rightCount
		}
		return out[i] < out[j]
	})
	return out
}

func containsOrderedString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func sourceRefBase(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if before, _, ok := strings.Cut(ref, "#"); ok {
		return before
	}
	return ref
}

func deriveSubjectExperienceLessons(subject string, horizonMemories []memory.SubjectHorizonMemory) []memory.SubjectExperienceLesson {
	lessons := make([]memory.SubjectExperienceLesson, 0)
	drivers := map[string]*subjectDriverExperience{}
	trends := map[string][]string{}
	for _, item := range horizonMemories {
		if strings.TrimSpace(item.TrendDirection) != "" {
			trends[item.TrendDirection] = append(trends[item.TrendDirection], item.Horizon)
		}
		for _, driver := range item.DriverClusters {
			driverSubject := strings.TrimSpace(driver.Subject)
			if driverSubject == "" {
				continue
			}
			entry := drivers[driverSubject]
			if entry == nil {
				entry = &subjectDriverExperience{subject: driverSubject}
				drivers[driverSubject] = entry
			}
			entry.count += driver.Count
			entry.horizons = orderedUniqueStrings(append(entry.horizons, item.Horizon))
			entry.changes = uniqueStrings(append(entry.changes, driver.Changes...))
			entry.paths = uniqueStrings(append(entry.paths, driver.RelationPaths...))
			entry.sourceRefs = uniqueStrings(append(entry.sourceRefs, driver.SourceRefs...))
		}
	}
	driverList := make([]*subjectDriverExperience, 0, len(drivers))
	for _, driver := range drivers {
		driverList = append(driverList, driver)
	}
	sort.SliceStable(driverList, func(i, j int) bool {
		if driverList[i].count != driverList[j].count {
			return driverList[i].count > driverList[j].count
		}
		return driverList[i].subject < driverList[j].subject
	})
	for _, driver := range driverList {
		if driver.count < 2 && len(driver.horizons) < 2 {
			continue
		}
		lesson := memory.SubjectExperienceLesson{
			ID:                 "driver:" + driver.subject,
			Kind:               "driver-pattern",
			Statement:          subjectExperienceStatement(subject, driver),
			Trigger:            subjectExperienceTrigger(subject, driver),
			Mechanism:          subjectExperienceMechanism(subject, driver),
			Implication:        subjectExperienceImplication(subject, driver),
			Boundary:           subjectExperienceBoundary(subject, driver),
			TransferRule:       subjectExperienceTransferRule(subject, driver),
			TimeScaleMeaning:   subjectExperienceTimeScaleMeaning(driver.horizons),
			Confidence:         subjectExperienceConfidence(driver.count, len(driver.horizons)),
			SupportCount:       driver.count,
			Horizons:           append([]string(nil), driver.horizons...),
			DriverSubjects:     []string{driver.subject},
			EvidenceSourceRefs: append([]string(nil), driver.sourceRefs...),
		}
		lessons = append(lessons, lesson)
	}
	if len(trends) > 1 {
		parts := make([]string, 0, len(trends))
		for trend, trendHorizons := range trends {
			parts = append(parts, trend+"@"+strings.Join(trendHorizons, "/"))
		}
		sort.Strings(parts)
		lessons = append(lessons, memory.SubjectExperienceLesson{
			ID:           "horizon:trend-shift",
			Kind:         "horizon-shift",
			Statement:    fmt.Sprintf("%s 的经验会随时间尺度改变：%s。短窗口结论不能直接外推到长窗口。", subject, strings.Join(parts, ", ")),
			Implication:  "读取经验时必须带 horizon；否则会把阶段性波动误认为长期规律。",
			Confidence:   0.6,
			SupportCount: len(horizonMemories),
			Horizons:     subjectExperienceAllHorizons(horizonMemories),
		})
	}
	return lessons
}

func subjectExperienceStatement(subject string, driver *subjectDriverExperience) string {
	return fmt.Sprintf("%s 会改变市场解释 %s 的状态，而不只是单次涨跌原因。", driver.subject, subject)
}

func subjectExperienceTrigger(subject string, driver *subjectDriverExperience) string {
	return fmt.Sprintf("%s 方向变化或预期差扩大时。", driver.subject)
}

func subjectExperienceMechanism(subject string, driver *subjectDriverExperience) string {
	if len(driver.paths) > 0 {
		path := driver.paths[0]
		if subjectExperiencePathNodeCount(path) <= 2 {
			return "记忆路径: " + path + "；中间机制未展开。"
		}
		return "记忆路径: " + path + "。"
	}
	return fmt.Sprintf("记忆显示 %s 与 %s 变化反复共同出现；它和其他因素的关系未建立。", driver.subject, subject)
}

func subjectExperienceImplication(subject string, driver *subjectDriverExperience) string {
	return fmt.Sprintf("观察 %s 时，先判断 %s 是短期噪声、阶段性约束，还是跨尺度状态变量；只有第三种才值得写成可迁移经验。", subject, driver.subject)
}

func subjectExperienceBoundary(subject string, driver *subjectDriverExperience) string {
	if len(driver.paths) == 0 {
		return "没有路径证据时，只能当作观察线索，不能当作因果解释。"
	}
	return "当前只按记忆路径解释；路径之外的中间因素不能自动补全。"
}

func subjectExperienceTransferRule(subject string, driver *subjectDriverExperience) string {
	return fmt.Sprintf("下次先找 %s -> ... -> %s 的路径证据；没有就只保留为线索。", driver.subject, subject)
}

func subjectExperiencePathNodeCount(path string) int {
	if strings.TrimSpace(path) == "" {
		return 0
	}
	count := 0
	for _, part := range strings.Split(path, "->") {
		if strings.TrimSpace(part) != "" {
			count++
		}
	}
	return count
}

func subjectExperienceTimeScaleMeaning(horizons []string) string {
	switch {
	case containsOrderedHorizon(horizons, "1w") && containsOrderedHorizon(horizons, "1m"):
		return "时间尺度含义：1w 说明它正在解释近期波动，1m 说明它不是单日噪声；若继续进入 1q/1y，才可升级为结构性经验。"
	case containsOrderedHorizon(horizons, "1q") || containsOrderedHorizon(horizons, "1y") || containsOrderedHorizon(horizons, "2y") || containsOrderedHorizon(horizons, "5y"):
		return "时间尺度含义：它已跨过短期噪声窗口，更可能是状态变量；更新频率可以降低，但反例权重应该提高。"
	default:
		return "时间尺度含义：当前主要是短窗口经验，只能指导近期观察，不能直接外推。"
	}
}

func subjectExperienceChangeHint(changes []string) string {
	if len(changes) == 0 {
		return ""
	}
	limit := 2
	if len(changes) < limit {
		limit = len(changes)
	}
	return " 支撑变化：" + strings.Join(changes[:limit], "；") + "。"
}

func containsOrderedHorizon(horizons []string, want string) bool {
	for _, horizon := range horizons {
		if horizon == want {
			return true
		}
	}
	return false
}

func subjectExperienceAllHorizons(horizonMemories []memory.SubjectHorizonMemory) []string {
	out := make([]string, 0, len(horizonMemories))
	for _, item := range horizonMemories {
		out = append(out, item.Horizon)
	}
	return orderedUniqueStrings(out)
}

func orderedUniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func subjectExperienceConfidence(count, horizonCount int) float64 {
	confidence := 0.45 + float64(count)*0.08 + float64(horizonCount)*0.08
	if confidence > 0.9 {
		return 0.9
	}
	return confidence
}

func summarizeSubjectExperience(out memory.SubjectExperienceMemory) string {
	subject := firstTrimmed(out.CanonicalSubject, out.Subject, "subject")
	if len(out.Lessons) == 0 {
		return fmt.Sprintf("%s 在 %s 时间尺度下还没有可复用经验。", subject, strings.Join(out.Horizons, "/"))
	}
	top := out.Lessons[0]
	return fmt.Sprintf("%s 在 %s 时间尺度下形成了 %d 条可复用经验；最强经验：%s", subject, strings.Join(out.Horizons, "/"), len(out.Lessons), top.Statement)
}

func subjectExperienceInputHash(horizonMemories []memory.SubjectHorizonMemory) string {
	inputs := make([]struct {
		Horizon   string
		InputHash string
		WindowEnd string
	}, 0, len(horizonMemories))
	for _, item := range horizonMemories {
		inputs = append(inputs, struct {
			Horizon   string
			InputHash string
			WindowEnd string
		}{item.Horizon, item.InputHash, item.WindowEnd})
	}
	payload, _ := json.Marshal(inputs)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func (s *SQLiteStore) getCachedSubjectExperienceMemory(ctx context.Context, userID, canonicalSubject, horizonSet, inputHash string) (memory.SubjectExperienceMemory, bool, error) {
	var payload, storedHash string
	err := s.db.QueryRowContext(ctx, `SELECT input_hash, payload_json FROM subject_experience_memories WHERE user_id = ? AND canonical_subject = ? AND horizon_set = ?`, userID, canonicalSubject, horizonSet).Scan(&storedHash, &payload)
	if err == sql.ErrNoRows {
		return memory.SubjectExperienceMemory{}, false, nil
	}
	if err != nil {
		return memory.SubjectExperienceMemory{}, false, err
	}
	if storedHash != inputHash {
		return memory.SubjectExperienceMemory{}, false, nil
	}
	var out memory.SubjectExperienceMemory
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return memory.SubjectExperienceMemory{}, false, fmt.Errorf("decode subject experience memory: %w", err)
	}
	out.CacheStatus = "fresh"
	return out, true, nil
}

func (s *SQLiteStore) upsertSubjectExperienceMemory(ctx context.Context, horizonSet string, out memory.SubjectExperienceMemory) error {
	payload, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO subject_experience_memories(user_id, subject, canonical_subject, horizon_set, input_hash, payload_json, generated_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, canonical_subject, horizon_set) DO UPDATE SET
		  subject = excluded.subject,
		  input_hash = excluded.input_hash,
		  payload_json = excluded.payload_json,
		  generated_at = excluded.generated_at,
		  updated_at = excluded.updated_at`,
		out.UserID,
		out.Subject,
		out.CanonicalSubject,
		horizonSet,
		out.InputHash,
		string(payload),
		out.GeneratedAt,
		out.GeneratedAt,
	)
	return err
}
