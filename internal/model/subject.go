package model

import (
	"strings"
)

// NATS 主题中 '.' 是层级分隔符，'*'、'>' 是通配符，空白字符不允许出现。
// 设备 ID 或点位 key 中如果包含这些字符，直接拼接主题会导致层级错乱、
// 订阅匹配错误。这里提供可逆的转义编码：
//
//	'~' -> "~7E"   '.' -> "~2E"   '*' -> "~2A"   '>' -> "~3E"
//	' ' -> "~20"   '\t' -> "~09"  '\r' -> "~0D"  '\n' -> "~0A"
//
// 不含这些字符的 token 编码后保持不变，因此对现有的普通设备 ID 完全兼容。

const hexDigits = "0123456789ABCDEF"

func needsEscape(c byte) bool {
	switch c {
	case '~', '.', '*', '>', ' ', '\t', '\r', '\n':
		return true
	}
	return false
}

// EncodeSubjectToken 将任意字符串编码为合法的单个 NATS 主题 token。
// 空字符串编码为 "_"，保证主题层级数量固定。
func EncodeSubjectToken(s string) string {
	if s == "" {
		return "_"
	}
	escape := false
	for i := 0; i < len(s); i++ {
		if needsEscape(s[i]) {
			escape = true
			break
		}
	}
	if !escape {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if needsEscape(c) {
			b.WriteByte('~')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0x0F])
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	}
	return 0, false
}

// DecodeSubjectToken 是 EncodeSubjectToken 的逆操作。无法解析的转义序列按原样保留。
func DecodeSubjectToken(s string) string {
	if s == "_" {
		return ""
	}
	if strings.IndexByte(s, '~') < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '~' && i+2 < len(s) {
			hi, ok1 := unhex(s[i+1])
			lo, ok2 := unhex(s[i+2])
			if ok1 && ok2 {
				b.WriteByte(hi<<4 | lo)
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// DataSubject 返回数据点发布到总线的主题：iot.data.<device>.<key>
func DataSubject(deviceID, key string) string {
	return "iot.data." + EncodeSubjectToken(deviceID) + "." + EncodeSubjectToken(key)
}

// RuleOutputSubject 返回规则输出主题：iot.rules.<kind>.<device>.<key>
// 规则产生的数据（转发、聚合结果等）统一发布到 iot.rules.> 之下，
// 不会被订阅 iot.data.> 的规则引擎再次消费，从而避免处理环路。
func RuleOutputSubject(kind, deviceID, key string) string {
	return "iot.rules." + EncodeSubjectToken(kind) + "." + EncodeSubjectToken(deviceID) + "." + EncodeSubjectToken(key)
}
