package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/kumaloha/VariX/varix/memory"
)

func runMemorySubjectTimeline(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory subject-timeline", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	subject := fs.String("subject", "", "subject or alias")
	card := fs.Bool("card", false, "render a readable subject timeline card view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" || strings.TrimSpace(*subject) == "" {
		fmt.Fprintln(stderr, "usage: varix memory subject-timeline --user <user_id> --subject <subject>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	timeline, err := store.BuildSubjectTimeline(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*subject), currentUTC())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *card {
		fmt.Fprint(stdout, formatSubjectTimelineCard(timeline))
		return 0
	}
	payload, err := json.MarshalIndent(timeline, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemorySubjectHorizon(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory subject-horizon", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	subject := fs.String("subject", "", "subject or alias")
	horizon := fs.String("horizon", "1w", "horizon: 1w, 1m, 1q, 1y, 2y, 5y")
	refresh := fs.Bool("refresh", false, "force recomputing this subject horizon memory")
	card := fs.Bool("card", false, "render a readable subject horizon card view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" || strings.TrimSpace(*subject) == "" {
		fmt.Fprintln(stderr, "usage: varix memory subject-horizon --user <user_id> --subject <subject> --horizon <1w|1m|1q|1y|2y|5y>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.GetSubjectHorizonMemory(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*subject), strings.TrimSpace(*horizon), currentUTC(), *refresh)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *card {
		fmt.Fprint(stdout, formatSubjectHorizonCard(out))
		return 0
	}
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemorySubjectExperience(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory subject-experience", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	subject := fs.String("subject", "", "subject or alias")
	horizons := fs.String("horizons", "1w,1m,1q,1y,2y,5y", "comma-separated horizons: 1w,1m,1q,1y,2y,5y")
	refresh := fs.Bool("refresh", false, "force recomputing subject horizon inputs and experience memory")
	card := fs.Bool("card", false, "render a readable subject experience card view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" || strings.TrimSpace(*subject) == "" {
		fmt.Fprintln(stderr, "usage: varix memory subject-experience --user <user_id> --subject <subject> --horizons <1w,1m,1q,1y,2y,5y>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.GetSubjectExperienceMemory(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*subject), splitMemoryHorizons(*horizons), currentUTC(), *refresh)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *card {
		fmt.Fprint(stdout, formatSubjectExperienceCard(out))
		return 0
	}
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func splitMemoryHorizons(value string) []string {
	parts := strings.Split(strings.TrimSpace(value), ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func formatSubjectTimelineCard(timeline memory.SubjectTimeline) string {
	var b strings.Builder
	subject := strings.TrimSpace(timeline.CanonicalSubject)
	if subject == "" {
		subject = strings.TrimSpace(timeline.Subject)
	}
	fmt.Fprintf(&b, "Subject Timeline\n- Subject: %s\n- Changes: %d\n", subject, len(timeline.Entries))
	if strings.TrimSpace(timeline.Summary) != "" {
		fmt.Fprintf(&b, "- Summary: %s\n", timeline.Summary)
	}
	for _, entry := range timeline.Entries {
		when := subjectTimelineCardWhen(entry)
		if when == "" {
			when = "timeless"
		}
		fmt.Fprintf(&b, "\nChange\n- When: %s\n- Change: %s\n- Role: %s primary=%t\n- Relation: %s\n- Structure: %s\n- Source: %s:%s#%s\n", when, entry.ChangeText, entry.GraphRole, entry.IsPrimary, entry.RelationToPrior, entry.Structure, entry.SourcePlatform, entry.SourceExternalID, entry.NodeID)
		if strings.TrimSpace(entry.VerificationStatus) != "" {
			fmt.Fprintf(&b, "- Verification: %s", entry.VerificationStatus)
			if strings.TrimSpace(entry.VerificationReason) != "" {
				fmt.Fprintf(&b, " (%s)", entry.VerificationReason)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

func subjectTimelineCardWhen(entry memory.SubjectChangeEntry) string {
	return firstNonEmpty(entry.TimeStart, entry.TimeEnd, entry.VerificationAsOf, entry.SourceCompiledAt, entry.SourceUpdatedAt, entry.TimeText, entry.TimeBucket)
}

func formatSubjectHorizonCard(out memory.SubjectHorizonMemory) string {
	var b strings.Builder
	subject := firstNonEmpty(strings.TrimSpace(out.CanonicalSubject), strings.TrimSpace(out.Subject))
	fmt.Fprintf(&b, "Subject Horizon\n- Subject: %s\n- Horizon: %s\n- Window: %s -> %s\n- Policy: %s next=%s\n- Cache: %s\n- Changes: %d sources=%d\n", subject, out.Horizon, out.WindowStart, out.WindowEnd, out.RefreshPolicy, out.NextRefreshAt, out.CacheStatus, out.SampleCount, out.SourceCount)
	if strings.TrimSpace(out.DominantPattern) != "" {
		fmt.Fprintf(&b, "- Pattern: %s\n", out.DominantPattern)
	}
	if strings.TrimSpace(out.Abstraction) != "" {
		fmt.Fprintf(&b, "- Abstraction: %s\n", out.Abstraction)
	}
	if len(out.DriverClusters) > 0 {
		driverParts := make([]string, 0, len(out.DriverClusters))
		for _, driver := range out.DriverClusters {
			driverParts = append(driverParts, fmt.Sprintf("%s(%d)", driver.Subject, driver.Count))
		}
		fmt.Fprintf(&b, "- Key factors: %s\n", strings.Join(driverParts, ", "))
	}
	for _, change := range out.KeyChanges {
		fmt.Fprintf(&b, "\nChange\n- When: %s\n- Change: %s\n- Relation: %s\n- Source: %s:%s#%s\n", firstNonEmpty(change.When, "timeless"), change.ChangeText, change.RelationToPrior, change.SourcePlatform, change.SourceExternalID, change.NodeID)
	}
	b.WriteString("\n")
	return b.String()
}

func formatSubjectExperienceCard(out memory.SubjectExperienceMemory) string {
	var b strings.Builder
	subject := firstNonEmpty(strings.TrimSpace(out.CanonicalSubject), strings.TrimSpace(out.Subject))
	fmt.Fprintf(&b, "主体归因总结\n- 主体: %s\n- 观察窗口: %s\n", subject, formatRecentHorizons(out.Horizons))
	if out.AttributionSummary.ChangeCount > 0 || out.AttributionSummary.FactorCount > 0 {
		fmt.Fprintf(&b, "- 变化数: %d\n- 因素数: %d\n", out.AttributionSummary.ChangeCount, out.AttributionSummary.FactorCount)
		if scope := subjectAttributionEvidenceScope(out); scope != "" {
			fmt.Fprintf(&b, "- 证据范围: %s\n", scope)
		}
		if note := subjectAttributionHorizonNote(out); note != "" {
			fmt.Fprintf(&b, "- 窗口提示: %s\n", note)
		}
	}
	if len(out.HorizonSummaries) > 0 {
		for _, summary := range out.HorizonSummaries {
			fmt.Fprintf(&b, "- %s: 样本=%d 趋势=%s 波动=%s", summary.Horizon, summary.SampleCount, summary.TrendDirection, summary.VolatilityState)
			if len(summary.TopDrivers) > 0 {
				fmt.Fprintf(&b, " 关键因素=%s", strings.Join(summary.TopDrivers, ", "))
			}
			b.WriteString("\n")
		}
	}
	if out.AttributionSummary.ChangeCount > 0 || strings.TrimSpace(out.AttributionSummary.PrimaryFactor.Subject) != "" {
		fmt.Fprintf(&b, "\n归因总结\n")
		if strings.TrimSpace(out.AttributionSummary.PrimaryFactor.Subject) != "" {
			primary := out.AttributionSummary.PrimaryFactor
			fmt.Fprintf(&b, "- 当前样本主要因素: %s（%d 个来源，support=%d", primary.Subject, primary.SourceCount, primary.Support)
			if len(primary.Horizons) > 0 {
				fmt.Fprintf(&b, "，%s", strings.Join(primary.Horizons, "/"))
			}
			b.WriteString("）\n")
			if strings.TrimSpace(primary.Reason) != "" {
				fmt.Fprintf(&b, "- 判断: %s\n", primary.Reason)
			}
		}
		if len(out.AttributionSummary.ChangeAttributions) > 0 {
			b.WriteString("- 变化归因:\n")
			for _, item := range out.AttributionSummary.ChangeAttributions {
				when := item.When
				if len(when) >= len("2006-01-02") {
					when = when[:len("2006-01-02")]
				}
				if strings.TrimSpace(when) == "" {
					when = "时间未知"
				}
				factors := "暂无"
				if len(item.Factors) > 0 {
					factors = strings.Join(item.Factors, ", ")
				}
				fmt.Fprintf(&b, "  - %s %s <= %s\n", when, item.ChangeText, factors)
			}
		}
		if len(out.AttributionSummary.FactorRelations) > 0 {
			b.WriteString("- 因素关系:\n")
			for _, relation := range out.AttributionSummary.FactorRelations {
				factors := relation.Factors
				if len(factors) == 0 {
					factors = []string{relation.Left, relation.Right}
				}
				fmt.Fprintf(&b, "  - %s: %s（%d 个来源）\n", strings.Join(factors, " + "), relation.Relation, relation.SourceCount)
			}
		} else {
			b.WriteString("- 因素关系: 暂无多因素共同变化\n")
		}
		return b.String()
	}
	for _, lesson := range out.Lessons {
		fmt.Fprintf(&b, "\n经验\n- 类型: %s\n- 结论: %s\n- 触发条件: %s\n", subjectExperienceKindLabel(lesson.Kind), lesson.Statement, lesson.Trigger)
		if strings.TrimSpace(lesson.Mechanism) != "" {
			fmt.Fprintf(&b, "- 作用机制: %s\n", lesson.Mechanism)
		}
		if strings.TrimSpace(lesson.Boundary) != "" {
			fmt.Fprintf(&b, "- 适用边界: %s\n", lesson.Boundary)
		}
		if strings.TrimSpace(lesson.TransferRule) != "" {
			fmt.Fprintf(&b, "- 迁移判断: %s\n", lesson.TransferRule)
		}
		fmt.Fprintf(&b, "- 置信度: %.2f support=%d\n", lesson.Confidence, lesson.SupportCount)
		if len(lesson.Horizons) > 0 {
			fmt.Fprintf(&b, "- 时间尺度: %s\n", strings.Join(lesson.Horizons, ", "))
		}
		if len(lesson.DriverSubjects) > 0 {
			fmt.Fprintf(&b, "- 关键因素: %s\n", strings.Join(lesson.DriverSubjects, ", "))
		}
		if len(lesson.EvidenceSourceRefs) > 0 {
			fmt.Fprintf(&b, "- 证据来源: %s\n", strings.Join(formatExperienceEvidenceRefs(lesson.EvidenceSourceRefs), ", "))
		}
	}
	b.WriteString("\n")
	return b.String()
}

func subjectAttributionEvidenceScope(out memory.SubjectExperienceMemory) string {
	items := out.AttributionSummary.ChangeAttributions
	if len(items) == 0 {
		return ""
	}
	start := strings.TrimSpace(items[0].When)
	end := start
	for _, item := range items[1:] {
		when := strings.TrimSpace(item.When)
		if when == "" {
			continue
		}
		if start == "" || when < start {
			start = when
		}
		if end == "" || when > end {
			end = when
		}
	}
	start = datePrefix(start)
	end = datePrefix(end)
	if start == "" && end == "" {
		return fmt.Sprintf("仅基于当前 %d 条变化样本", len(items))
	}
	if start == end {
		return fmt.Sprintf("%s，基于当前 %d 条变化样本", start, len(items))
	}
	return fmt.Sprintf("%s 至 %s，基于当前 %d 条变化样本", start, end, len(items))
}

func subjectAttributionHorizonNote(out memory.SubjectExperienceMemory) string {
	if len(out.Horizons) <= 1 || len(out.HorizonSummaries) <= 1 {
		return ""
	}
	if attributionHorizonsUseSameSamples(out) {
		return fmt.Sprintf("%s 目前使用同一批样本，只能代表这批证据，不能推出更长窗口的主导因素。", formatRecentHorizons(out.Horizons))
	}
	return "不同观察窗口的主导因素需要分别读取，不能把较短窗口的结论直接套到较长窗口。"
}

func formatRecentHorizons(horizons []string) string {
	if len(horizons) == 0 {
		return ""
	}
	out := make([]string, 0, len(horizons))
	for _, horizon := range horizons {
		horizon = strings.TrimSpace(horizon)
		if horizon == "" {
			continue
		}
		out = append(out, "最近 "+horizon)
	}
	return strings.Join(out, ", ")
}

func attributionHorizonsUseSameSamples(out memory.SubjectExperienceMemory) bool {
	if len(out.HorizonSummaries) == 0 {
		return false
	}
	want := out.HorizonSummaries[0].SampleCount
	if want == 0 {
		return false
	}
	for _, summary := range out.HorizonSummaries[1:] {
		if summary.SampleCount != want {
			return false
		}
	}
	return want == out.AttributionSummary.ChangeCount
}

func datePrefix(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= len("2006-01-02") {
		return value[:len("2006-01-02")]
	}
	return value
}

func subjectExperienceKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case "driver-pattern":
		return "可复用解释因素"
	case "horizon-shift":
		return "时间尺度变化"
	default:
		return strings.TrimSpace(kind)
	}
}

func formatExperienceEvidenceRefs(refs []string) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		source := strings.TrimSpace(ref)
		if before, _, ok := strings.Cut(source, "#"); ok {
			source = before
		}
		if source != "" {
			out = append(out, source)
		}
	}
	return uniqueCLIStrings(out)
}
