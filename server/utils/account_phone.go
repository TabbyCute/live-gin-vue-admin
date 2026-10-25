package utils

import (
	"errors"
	"regexp"
	"strings"
)

var accountPhonePattern = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

// NormalizeAccountPhone 只接受携带国家码的统一格式，不猜测本地号码所属国家。
func NormalizeAccountPhone(value string) (string, error) {
	phone := strings.TrimSpace(value)
	if !accountPhonePattern.MatchString(phone) {
		return "", errors.New("手机号必须使用E.164格式，例如+60123456789")
	}
	return phone, nil
}
