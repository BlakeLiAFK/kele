package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronExpr 解析后的 cron 表达式
type CronExpr struct {
	Minutes     []bool // [0..59]
	Hours       []bool // [0..23]
	DaysOfMonth []bool // [0..31], index 0 未使用
	Months      []bool // [0..12], index 0 未使用
	DaysOfWeek  []bool // [0..6], Sunday=0
	Raw         string
}

// 预定义快捷方式
var shortcuts = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

// Parse 解析 cron 表达式（5字段标准格式）
// 格式：分 时 日 月 周
// 支持：* */N N N-M N,M 及其组合
func Parse(expr string) (*CronExpr, error) {
	expr = strings.TrimSpace(expr)

	// 处理快捷方式
	if replacement, ok := shortcuts[strings.ToLower(expr)]; ok {
		expr = replacement
	}

	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron 表达式需要 5 个字段，收到 %d 个: %q", len(fields), expr)
	}

	c := &CronExpr{
		Minutes:     make([]bool, 60),
		Hours:       make([]bool, 24),
		DaysOfMonth: make([]bool, 32),
		Months:      make([]bool, 13),
		DaysOfWeek:  make([]bool, 7),
		Raw:         expr,
	}

	if err := parseField(fields[0], c.Minutes, 0, 59); err != nil {
		return nil, fmt.Errorf("minute 字段错误: %v", err)
	}
	if err := parseField(fields[1], c.Hours, 0, 23); err != nil {
		return nil, fmt.Errorf("hour 字段错误: %v", err)
	}
	if err := parseField(fields[2], c.DaysOfMonth, 1, 31); err != nil {
		return nil, fmt.Errorf("day-of-month 字段错误: %v", err)
	}
	if err := parseField(fields[3], c.Months, 1, 12); err != nil {
		return nil, fmt.Errorf("month 字段错误: %v", err)
	}
	if err := parseField(fields[4], c.DaysOfWeek, 0, 6); err != nil {
		return nil, fmt.Errorf("day-of-week 字段错误: %v", err)
	}

	return c, nil
}

// Matches 检查给定时间是否匹配该 cron 表达式
func (c *CronExpr) Matches(t time.Time) bool {
	return c.Minutes[t.Minute()] &&
		c.Hours[t.Hour()] &&
		c.DaysOfMonth[t.Day()] &&
		c.Months[int(t.Month())] &&
		c.DaysOfWeek[int(t.Weekday())]
}

// NextAfter 计算指定时间之后的下一个匹配时刻
func (c *CronExpr) NextAfter(t time.Time) time.Time {
	t = t.Truncate(time.Minute).Add(time.Minute)
	limit := t.Add(366 * 24 * time.Hour)

	for t.Before(limit) {
		// 快速跳过不匹配的月
		if !c.Months[int(t.Month())] {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		// 快速跳过不匹配的日
		if !c.DaysOfMonth[t.Day()] || !c.DaysOfWeek[int(t.Weekday())] {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		// 快速跳过不匹配的时
		if !c.Hours[t.Hour()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		if c.Minutes[t.Minute()] {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}

// parseField 解析单个 cron 字段（逗号分隔的多个部分）
func parseField(field string, bits []bool, min, max int) error {
	for _, part := range strings.Split(field, ",") {
		if err := parsePart(strings.TrimSpace(part), bits, min, max); err != nil {
			return err
		}
	}
	return nil
}

// parsePart 解析逗号分隔的单个部分
func parsePart(part string, bits []bool, min, max int) error {
	// */N 全范围步进
	if strings.HasPrefix(part, "*/") {
		step, err := strconv.Atoi(part[2:])
		if err != nil || step <= 0 {
			return fmt.Errorf("无效步进: %s", part)
		}
		for i := min; i <= max; i += step {
			bits[i] = true
		}
		return nil
	}

	// * 通配
	if part == "*" {
		for i := min; i <= max; i++ {
			bits[i] = true
		}
		return nil
	}

	// N-M/S 范围+步进 或 N-M 范围
	if strings.Contains(part, "-") {
		rangeParts := strings.SplitN(part, "-", 2)
		step := 1
		endPart := rangeParts[1]

		if strings.Contains(endPart, "/") {
			stepParts := strings.SplitN(endPart, "/", 2)
			endPart = stepParts[0]
			var err error
			step, err = strconv.Atoi(stepParts[1])
			if err != nil || step <= 0 {
				return fmt.Errorf("无效步进: %s", part)
			}
		}

		start, err := strconv.Atoi(rangeParts[0])
		if err != nil {
			return fmt.Errorf("无效范围起始: %s", part)
		}
		end, err := strconv.Atoi(endPart)
		if err != nil {
			return fmt.Errorf("无效范围结束: %s", part)
		}
		if start < min || end > max || start > end {
			return fmt.Errorf("范围越界: %s (允许 %d-%d)", part, min, max)
		}

		for i := start; i <= end; i += step {
			bits[i] = true
		}
		return nil
	}

	// 单个数字
	val, err := strconv.Atoi(part)
	if err != nil {
		return fmt.Errorf("无效值: %s", part)
	}
	if val < min || val > max {
		return fmt.Errorf("值越界: %d (允许 %d-%d)", val, min, max)
	}
	bits[val] = true
	return nil
}
