package v2identity

import "github.com/google/uuid"

// Actor is the server-owned identity used to attribute business transactions.
type Actor struct {
	ID       uuid.UUID `json:"id"`
	Username string    `json:"username"`
	Roles    []string  `json:"roles"`
}

// PublicActor is the single fixed identity used by the portfolio demo. Login and
// account management are intentionally outside the demo scope.
var PublicActor = Actor{
	ID:       uuid.MustParse("00000000-0000-0000-0000-000000000002"),
	Username: "portfolio",
	Roles:    []string{"admin", "planner", "materials", "operator", "quality"},
}
