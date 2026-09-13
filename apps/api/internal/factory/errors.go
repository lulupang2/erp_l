package factory

import "example.com/assembly-erp/api/internal/domain"

func Forbidden() error {
	return &domain.Error{Status: 403, Code: "FORBIDDEN", Message: "이 작업을 수행할 권한이 없습니다."}
}

func Unauthorized() error {
	return &domain.Error{Status: 401, Code: "UNAUTHENTICATED", Message: "로그인이 필요하거나 세션이 만료되었습니다."}
}
