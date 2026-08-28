package auth

import (
	"strings"
	"testing"
)

func TestUsersLoadAndJoinByEmailCaseInsensitively(t *testing.T) {
	users := writeUsers(t, goodUsers)

	user, ok := users.ByEmail("JO@Example.COM")
	if !ok {
		t.Fatalf("ByEmail missed a known user: the join is case-insensitive")
	}
	if user.Name != "Jo Author" || user.Owner != "gateway-owners" {
		t.Fatalf("ByEmail returned %+v", user)
	}
	if got := users.Emails(); len(got) != 2 {
		t.Fatalf("Emails() = %v", got)
	}
}

func TestUsersLoadFailsClosed(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{
			"an owner outside the tree",
			"users:\n  - email: jo@example.com\n    name: Jo\n    owner: nobody\n",
			"not in the team tree",
		},
		{
			"a duplicate email",
			"users:\n  - email: jo@example.com\n    name: Jo\n    owner: gateway-owners\n  - email: JO@example.com\n    name: Jo Again\n    owner: pii-guardians\n",
			"appears twice",
		},
		{
			"a missing name",
			"users:\n  - email: jo@example.com\n    owner: gateway-owners\n",
			"has no name",
		},
		{
			"a missing owner",
			"users:\n  - email: jo@example.com\n    name: Jo\n",
			"names no owner",
		},
		{
			"a username that is not an email",
			"users:\n  - email: jo\n    name: Jo\n    owner: gateway-owners\n",
			"not an email address",
		},
		{
			"an unknown field",
			"users:\n  - email: jo@example.com\n    name: Jo\n    owner: gateway-owners\n    role: admin\n",
			"field role not found",
		},
		{
			"an empty users list",
			"users: []\n",
			"holds no users",
		},
		{
			"a malformed password hash",
			"users:\n  - email: jo@example.com\n    name: Jo\n    owner: gateway-owners\n    password: plaintext-secret\n",
			"telecraft passwd",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := usersErr(t, tc.body); !strings.Contains(got, tc.want) {
				t.Fatalf("error %q does not name the problem (want %q)", got, tc.want)
			}
		})
	}
}
