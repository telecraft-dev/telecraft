package register

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write lays out a register directory from file name to contents.
func write(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const acme = `name: acme
display_name: Acme Logistics
state: active
address: https://acme.telecraft.example
estate:
  kind: hosted
administrators:
  - oidc:8f21c0
`

const beacon = `name: beacon
display_name: Beacon Rail
state: active
address: https://beacon.telecraft.example
estate:
  kind: connected
  repository: https://git.example.com/beacon/estate.git
`

// The whole of what a register is: a directory of records, each naming one
// Organisation, the address it answers on, and where its estate comes from.
func TestARegisterLoadsTheRecordsItHolds(t *testing.T) {
	reg, err := Load(write(t, map[string]string{
		"beacon.yaml": beacon,
		"acme.yaml":   acme,
		"corvid.yaml": "name: corvid\nstate: retired\n",
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}

	var names []string
	for _, org := range reg.Organisations {
		names = append(names, org.Name)
	}
	if got, want := strings.Join(names, " "), "acme beacon corvid"; got != want {
		t.Errorf("the register holds %q, want %q in name order", got, want)
	}

	org, ok := reg.Lookup("beacon")
	if !ok {
		t.Fatal("beacon is not in the register it was written into")
	}
	if org.DisplayName != "Beacon Rail" || org.Address != "https://beacon.telecraft.example" {
		t.Errorf("beacon = %+v, want the record as authored", org)
	}
	if org.Estate.Kind != SourceConnected || org.Estate.Repository != "https://git.example.com/beacon/estate.git" {
		t.Errorf("beacon's estate = %+v, want the connected remote it names", org.Estate)
	}

	acmeOrg, _ := reg.Lookup("acme")
	if acmeOrg.Estate.Kind != SourceHosted || acmeOrg.Estate.Repository != "" {
		t.Errorf("acme's estate = %+v, want a hosted source naming no remote", acmeOrg.Estate)
	}
	if len(acmeOrg.Administrators) != 1 || acmeOrg.Administrators[0] != "oidc:8f21c0" {
		t.Errorf("acme's administrators = %v, want the one identity the record names", acmeOrg.Administrators)
	}

	active := reg.Active()
	if len(active) != 2 || active[0].Name != "acme" || active[1].Name != "beacon" {
		t.Errorf("the active set is %v, want the two the deployment runs an Instance for", active)
	}
}

// A name addresses an Organisation, so it belongs to one and is never
// issued twice. The retired record is what holds a retired name.
func TestANameIsNeverIssuedTwice(t *testing.T) {
	_, err := Load(write(t, map[string]string{
		"acme.yaml": acme,
		"acme.yml":  acme,
	}))
	if err == nil {
		t.Fatal("two records naming one Organisation were admitted")
	}
	if !strings.Contains(err.Error(), "never issued twice") {
		t.Errorf("the refusal does not say what the rule is: %v", err)
	}

	reg, err := Load(write(t, map[string]string{"corvid.yaml": "name: corvid\nstate: retired\n"}))
	if err != nil {
		t.Fatalf("a retired record was refused: %v", err)
	}
	if len(reg.Active()) != 0 {
		t.Error("a retired Organisation is in the active set")
	}
	if org, ok := reg.Lookup("corvid"); !ok || org.State != StateRetired {
		t.Errorf("corvid = %+v %v, want a retired record holding its name", org, ok)
	}
}

// Two Organisations at one address would send one Organisation's traffic
// to the other, and two reading one estate would be a read across the
// boundary the product is.
func TestNoTwoOrganisationsShareAnAddressOrAnEstate(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"one address": {
			"acme.yaml":   acme,
			"beacon.yaml": strings.Replace(beacon, "https://beacon.telecraft.example", "https://acme.telecraft.example", 1),
		},
		"one estate": {
			"beacon.yaml": beacon,
			"corvid.yaml": strings.Replace(strings.Replace(beacon, "name: beacon", "name: corvid", 1), "https://beacon.telecraft.example", "https://corvid.telecraft.example", 1),
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(write(t, files)); err == nil {
				t.Error("the register was admitted")
			}
		})
	}
}

// The register carries names and addresses. A credential cannot be written
// into one, and a field that would hold a secret value is not in the
// schema to write.
func TestTheRegisterNeverCarriesACredential(t *testing.T) {
	for name, body := range map[string]string{
		"a password in the remote": strings.Replace(beacon, "https://git.example.com/", "https://beacon:s3cret@git.example.com/", 1),
		"a password in the address": strings.Replace(beacon, "address: https://beacon.telecraft.example",
			"address: https://beacon:s3cret@beacon.telecraft.example", 1),
		"a field for a value": beacon + "client_secret: s3cret\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(write(t, map[string]string{"beacon.yaml": body}))
			if err == nil {
				t.Fatal("the record was admitted")
			}
			if strings.Contains(err.Error(), "s3cret") {
				t.Errorf("the refusal quotes the secret it refused: %v", err)
			}
		})
	}
}

// The name is an address, so it is bound by what an address allows, and
// the file is named for the Organisation so a review reads one record at a
// time.
func TestANameIsBoundByWhatAnAddressAllows(t *testing.T) {
	for name, tc := range map[string]struct {
		file, org string
		refused   bool
	}{
		"a plain name":         {file: "acme.yaml", org: "acme"},
		"digits and hyphens":   {file: "acme-2.yaml", org: "acme-2"},
		"capitals":             {file: "Acme.yaml", org: "Acme", refused: true},
		"a leading hyphen":     {file: "-acme.yaml", org: "-acme", refused: true},
		"a trailing hyphen":    {file: "acme-.yaml", org: "acme-", refused: true},
		"a dot":                {file: "acme.co.yaml", org: "acme.co", refused: true},
		"a path":               {file: "a-b.yaml", org: "a/b", refused: true},
		"longer than a label":  {file: "long.yaml", org: strings.Repeat("a", 64), refused: true},
		"a file of its own":    {file: "acme.yml", org: "acme"},
		"a file named another": {file: "other.yaml", org: "acme", refused: true},
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(acme, "name: acme", "name: "+tc.org, 1)
			_, err := Load(write(t, map[string]string{tc.file: body}))
			if tc.refused && err == nil {
				t.Errorf("%q in %s was admitted", tc.org, tc.file)
			}
			if !tc.refused && err != nil {
				t.Errorf("%q in %s was refused: %v", tc.org, tc.file, err)
			}
		})
	}
}

// A record that is still run for names what running it needs. The refusal
// says which file and what is missing, and every problem in the register
// is reported at once, because the register is reviewed as one change.
func TestAnActiveRecordNamesWhatRunningItNeeds(t *testing.T) {
	_, err := Load(write(t, map[string]string{
		"acme.yaml":   "name: acme\nstate: active\n",
		"beacon.yaml": strings.Replace(beacon, "  repository: https://git.example.com/beacon/estate.git\n", "", 1),
	}))
	if err == nil {
		t.Fatal("records naming neither an address nor a repository were admitted")
	}
	for _, want := range []string{"acme.yaml", "beacon.yaml", "address", "repository"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal never mentions %q:\n%v", want, err)
		}
	}
}

// A hosted repository is created with the Organisation, so naming one is
// naming something the register does not decide.
func TestAHostedEstateNamesNoRemote(t *testing.T) {
	body := strings.Replace(acme, "  kind: hosted\n", "  kind: hosted\n  repository: https://git.example.com/acme/estate.git\n", 1)
	if _, err := Load(write(t, map[string]string{"acme.yaml": body})); err == nil {
		t.Error("a hosted estate naming a remote was admitted")
	}
}

// A state the register does not hold is refused rather than read as one it
// does. Suspension is an open question, and a record that asked for it
// would be answering it.
func TestTheLifecycleStatesAreTheTwoTheRegisterHolds(t *testing.T) {
	for _, state := range []string{"suspended", "requested", ""} {
		body := strings.Replace(acme, "state: active", "state: "+state, 1)
		if _, err := Load(write(t, map[string]string{"acme.yaml": body})); err == nil {
			t.Errorf("the state %q was admitted", state)
		}
	}
}

// A deployment that serves no Organisation yet has an empty register, and
// loading one is not a failure: reconciling it plans nothing.
func TestAnEmptyRegisterLoads(t *testing.T) {
	reg, err := Load(write(t, map[string]string{"README.md": "The register.\n"}))
	if err != nil {
		t.Fatalf("an empty register was refused: %v", err)
	}
	if len(reg.Organisations) != 0 {
		t.Errorf("the register holds %d records, want none", len(reg.Organisations))
	}
	if _, err := Load(filepath.Join(t.TempDir(), "nowhere")); err == nil {
		t.Error("a register directory that does not exist was admitted")
	}
}
