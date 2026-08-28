package auth

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/telecraft-dev/telecraft/pkg/ownership"
	"gopkg.in/yaml.v3"
)

// UsersFile is the users seam beside teams.yaml in the estate ownership
// directory: the reviewable, git-resident mapping from authenticated
// identities to the Owner each acts as (ADR-0019; the ADR-0017 seam
// pattern). Air-gap first-class: the whole membership story lives in the
// estate repo, no directory service required.
//
// The group mapping in auth.yaml (see Groups) stands beside this file
// rather than replacing it. Both feed one resolution step, and this file
// wins wherever it names an email: a bulk rule must never quietly override
// a person somebody wrote down on purpose.
const UsersFile = "users.yaml"

// User is one signed-in human the estate knows: the email their provider
// asserts, the name that authors their changes when the provider carries
// none, and the Owner they act as.
type User struct {
	// Email joins the authenticated identity to this record, compared
	// case-insensitively. One email, one user.
	Email string `yaml:"email"`

	// Name attributes changes when the provider supplies no name claim:
	// basic auth always, OIDC without a profile scope.
	Name string `yaml:"name"`

	// Owner is the Owner this human acts as (ADR-0016). The Owner's Team
	// is what authorization derives from.
	Owner ownership.OwnerID `yaml:"owner"`

	// Password is the PBKDF2 hash basic auth verifies against, in the
	// HashSecret format. Empty means this user cannot sign in with basic
	// auth, the OIDC/SAML-only production shape (ADR-0019 §1).
	Password string `yaml:"password"`
}

// Users is the loaded, validated users.yaml.
type Users struct {
	byEmail map[string]User
}

// ByEmail returns the user an asserted email joins to, case-insensitively.
func (u Users) ByEmail(email string) (User, bool) {
	user, ok := u.byEmail[strings.ToLower(email)]
	return user, ok
}

// Emails lists every known email, sorted, for error messages and tests.
func (u Users) Emails() []string {
	out := make([]string, 0, len(u.byEmail))
	for e := range u.byEmail {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

// LoadUsers reads and validates users.yaml against the team tree. Loading
// fails closed, matching pkg/ownership: an unknown field, a user whose
// owner nobody's team contains, or a duplicate email is a load error naming
// the file. A human who authenticates but resolves to nobody would hold an
// unattributable session, and that is worse than a crash at start-up.
func LoadUsers(path string, tree ownership.Tree) (Users, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Users{}, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return Users{}, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return Users{}, fmt.Errorf("%s: the file is empty. Declare at least one user", path)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var file struct {
		Users []User `yaml:"users"`
	}
	if err := dec.Decode(&file); err != nil {
		return Users{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return Users{}, fmt.Errorf("%s: more than one YAML document in the file", path)
	}
	if len(file.Users) == 0 {
		return Users{}, fmt.Errorf("%s: holds no users. Declare at least one, or nobody can sign in", path)
	}

	users := Users{byEmail: map[string]User{}}
	var problems []string
	for _, u := range file.Users {
		email := strings.ToLower(strings.TrimSpace(u.Email))
		switch {
		case email == "":
			problems = append(problems, "a user has no email. The email is how a signed-in identity matches its user")
			continue
		case !strings.Contains(email, "@"):
			problems = append(problems, fmt.Sprintf("user %q: not an email address", u.Email))
			continue
		}
		if _, dup := users.byEmail[email]; dup {
			problems = append(problems, fmt.Sprintf("user %q appears twice. Each email belongs to one user", email))
			continue
		}
		if u.Name == "" {
			problems = append(problems, fmt.Sprintf("user %q has no name. Every user needs a name to author changes", email))
		}
		if u.Owner == "" {
			problems = append(problems, fmt.Sprintf("user %q names no owner. The owner decides what the user may author", email))
		} else if _, known := tree.Owners[u.Owner]; !known {
			problems = append(problems, fmt.Sprintf("user %q acts as owner %q, which is not in the team tree", email, u.Owner))
		}
		if u.Password != "" {
			if err := checkHashFormat(u.Password); err != nil {
				problems = append(problems, fmt.Sprintf("user %q: %v", email, err))
			}
		}
		u.Email = email
		users.byEmail[email] = u
	}
	if len(problems) > 0 {
		return Users{}, fmt.Errorf("invalid %s:\n  - %s", path, strings.Join(problems, "\n  - "))
	}
	return users, nil
}
