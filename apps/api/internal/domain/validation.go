package domain

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

var codePattern = regexp.MustCompile(`^[A-Z0-9_-]{1,40}$`)
var idPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func ID(value string) (uuid.UUID, error) {
	if !idPattern.MatchString(value) {
		return uuid.Nil, Input("올바른 UUID 식별자가 필요합니다.")
	}
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, Input("올바른 UUID 식별자가 필요합니다.")
	}
	return id, nil
}

func Text(value string, min, max int, label string) (string, error) {
	value = strings.TrimSpace(value)
	n := utf8.RuneCountInString(value)
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) || n < min || n > max {
		return "", Input(label + "의 길이 또는 내용이 올바르지 않습니다.")
	}
	return value, nil
}

func NormalizeItem(in ItemInput) (ItemInput, error) {
	in.Code = strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, in.Code))
	if !codePattern.MatchString(in.Code) {
		return in, Input("품목 코드는 영문·숫자·_·-로 1~40자여야 합니다.")
	}
	var err error
	if in.Name, err = Text(in.Name, 1, 100, "품목명"); err != nil {
		return in, err
	}
	if in.Unit, err = Text(in.Unit, 1, 20, "단위"); err != nil {
		return in, err
	}
	if in.Kind != "component" && in.Kind != "finished_good" {
		return in, Input("품목 종류가 올바르지 않습니다.")
	}
	return in, nil
}

func Quantity(value, minimum int64) error {
	if value < minimum || value > MaxInput {
		return Input("수량은 허용 범위의 정수여야 합니다.")
	}
	return nil
}

func ValidateQuery(q Query) error {
	if q.Page < 1 || q.Page > 1_000_000_000 || q.PageSize < 1 || q.PageSize > 100 {
		return Input("페이지는 1 이상, 페이지 크기는 1~100이어야 합니다.")
	}
	if q.Kind != "" && q.Kind != "component" && q.Kind != "finished_good" {
		return Input("품목 종류 필터가 올바르지 않습니다.")
	}
	if q.Status != "" && q.Status != "pending" && q.Status != "in_progress" && q.Status != "completed" {
		return Input("생산 상태 필터가 올바르지 않습니다.")
	}
	_, err := Text(q.Search, 0, 1000, "검색어")
	return err
}
