package model

import (
	"strings"
)

func SplitParallelDrivers(drivers []string) []string {
	out := make([]string, 0, len(drivers))
	for _, driver := range drivers {
		parts := splitOneParallelDriver(driver)
		if len(parts) == 0 {
			continue
		}
		out = append(out, parts...)
	}
	return dedupeNormalizedStrings(out)
}

func splitParallelDrivers(drivers []string) []string {
	return SplitParallelDrivers(drivers)
}
func splitOneParallelDriver(driver string) []string {
	driver = strings.TrimSpace(driver)
	if driver == "" {
		return nil
	}
	connectors := []string{"以及", "并且", "同时", "和", "及", "与", "且"}
	for _, connector := range connectors {
		if !strings.Contains(driver, connector) {
			continue
		}
		parts := strings.Split(driver, connector)
		if len(parts) < 2 {
			continue
		}
		clean := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(strings.Trim(part, "，,;；。"))
			if part == "" {
				continue
			}
			clean = append(clean, part)
		}
		if len(clean) >= 2 && allClauseLikeNodes(clean) {
			return clean
		}
	}
	return []string{driver}
}
func dedupeNormalizedStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
func allClauseLikeNodes(items []string) bool {
	for _, item := range items {
		if !looksNodeLikeClause(item) {
			return false
		}
	}
	return true
}
func looksNodeLikeClause(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	changeMarkers := []string{
		"上升", "下降", "回落", "走高", "走低", "扩大", "收窄", "恢复", "恶化", "改善", "紧张", "缓和", "维持", "增加", "减少", "进入", "脱离", "消退", "定价", "反弹", "承压", "上行", "下行", "高企", "亮红灯", "耗竭", "消耗", "放大", "抬升", "压低", "改善", "恶化", "预期", "偏好", "吸引力", "风险", "脆弱", "可控",
		"rise", "fall", "drop", "decline", "increase", "decrease", "expand", "narrow", "recover", "deteriorate", "improve", "tighten", "ease", "remain", "maintain", "rebound", "price", "priced", "price in", "overbought", "oversold",
	}
	for _, marker := range changeMarkers {
		if strings.Contains(strings.ToLower(text), strings.ToLower(marker)) {
			return true
		}
	}
	return false
}
