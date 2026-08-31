package bundle

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFixedContractHasExactlyTwelveOneToOneActivities(t *testing.T) {
	root := filepath.Join("..", "..")
	intent, err := ParseIntent(filepath.Join(root, "examples", "change-bundle", "change-intent.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	contract, err := LoadContract(filepath.Join(root, "contracts", "change-bundle-denominator-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateDeclarations(intent, contract); err != nil {
		t.Fatal(err)
	}
}

func TestFixtureOracleHasAtLeastThreeCasesPerResolutionClass(t *testing.T) {
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, "fixtures", "scenarios.json"))
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Schema string `json:"schema"`
		Cases  []struct {
			Class    string `json:"class"`
			Expected string `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	if oracle.Schema != "gooo/change-bundle/fixture-oracle/v1" || len(oracle.Cases) != FixedCells {
		t.Fatalf("fixture oracle cardinality = %d", len(oracle.Cases))
	}
	counts := map[string]int{}
	for _, item := range oracle.Cases {
		counts[item.Class]++
		if item.Expected != item.Class && !(item.Class == "NORMAL" && item.Expected == DecisionClosed) {
			t.Fatalf("fixture %v has inconsistent expected state", item)
		}
	}
	if counts["NORMAL"] < 3 || counts["UNKNOWN"] < 3 || counts["REFUTED"] < 3 {
		t.Fatalf("fixture classes lack minimum cardinality: %+v", counts)
	}
}

func TestNormalMaterializationNeverWritesInputAndRoundTrips(t *testing.T) {
	root := testSource(t, "before\n")
	proposalPath, authorityPath := testApprovalChain(t, root, "app.txt", OperationModify, []byte("after\n"), nil)
	digest, err := ComputeSourceDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	result := runFixture(t, root, digest, proposalPath, authorityPath)
	if result.Manifest.Decision != DecisionClosed {
		t.Fatalf("decision = %s, findings = %+v", result.Manifest.Decision, result.Manifest.Findings)
	}
	if result.Manifest.Metrics.ReplayMismatches != 0 || result.Manifest.Metrics.RollbackMismatches != 0 {
		t.Fatalf("round-trip mismatches: %+v", result.Manifest.Metrics)
	}
	data, err := os.ReadFile(filepath.Join(root, "app.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "before\n" {
		t.Fatalf("input repository was changed: %q", data)
	}
	if _, ok := result.Files["bundle-manifest.json"]; !ok {
		t.Fatal("result did not retain the bundle manifest")
	}
}

func TestSafetyCasesFailClosed(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		operation string
		status    string
		hunks     []Hunk
		rollback  string
	}{
		{name: "path-traversal", path: "../escape.txt", operation: OperationAdd, status: DecisionRefuted},
		{name: "generated-file-authority", path: "generated/evaluator.go", operation: OperationAdd, status: DecisionRefuted},
		{name: "conflicting-hunks", path: "app.txt", operation: OperationModify, status: DecisionRefuted, hunks: []Hunk{{StartLine: 1, EndLine: 1}, {StartLine: 1, EndLine: 2}}},
		{name: "rollback-mismatch", path: "app.txt", operation: OperationModify, status: DecisionRefuted, rollback: "sha256:bad"},
		{name: "stale-preimage", path: "app.txt", operation: OperationModify, status: DecisionRefuted},
		{name: "unauthorized-proposal", path: "app.txt", operation: OperationModify, status: DecisionRefuted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := testSource(t, "before\n")
			proposalPath, authorityPath := testApprovalChain(t, root, tc.path, tc.operation, []byte("after\n"), tc.hunks)
			if tc.rollback != "" {
				mutateProposalRollback(t, proposalPath, tc.rollback)
			}
			digest, err := ComputeSourceDigest(root)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "stale-preimage" {
				digest = DigestBytes([]byte("stale"))
			}
			if tc.name == "unauthorized-proposal" {
				mutateProposalStatus(t, proposalPath, "DRAFT")
			}
			result := runFixture(t, root, digest, proposalPath, authorityPath)
			if result.Manifest.Decision != tc.status {
				t.Fatalf("decision = %s, findings = %+v", result.Manifest.Decision, result.Manifest.Findings)
			}
		})
	}
}

func TestSymlinkEscapeFailsClosedWithoutFollowingTheLink(t *testing.T) {
	root := testSource(t, "before\n")
	proposalPath, authorityPath := testApprovalChain(t, root, "app.txt", OperationModify, []byte("after\n"), nil)
	escapeTarget := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(escapeTarget, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(escapeTarget, filepath.Join(root, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	digest := DigestBytes([]byte("untrusted"))
	result := runFixture(t, root, digest, proposalPath, authorityPath)
	if result.Manifest.Decision != DecisionRefuted {
		t.Fatalf("symlink decision = %s, findings = %+v", result.Manifest.Decision, result.Manifest.Findings)
	}
	for _, finding := range result.Manifest.Findings {
		if finding.Code == "SYMLINK_ESCAPE" {
			return
		}
	}
	t.Fatal("symlink escape finding was not retained")
}

func TestUnknownCasesPreserveAllSixOperationalFields(t *testing.T) {
	cases := []string{"missing-source-digest", "missing-source-tree", "dependency-blocked"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			root := testSource(t, "before\n")
			proposalPath, authorityPath := testApprovalChain(t, root, "app.txt", OperationModify, []byte("after\n"), nil)
			digest, err := ComputeSourceDigest(root)
			if err != nil {
				t.Fatal(err)
			}
			if name == "missing-source-digest" {
				digest = ""
			}
			if name == "missing-source-tree" {
				root = filepath.Join(t.TempDir(), "missing")
			}
			if name == "dependency-blocked" {
				observationPath := filepath.Join(t.TempDir(), "observation.json")
				writeTestJSON(t, observationPath, observation{SourceTreeObservable: true, DependencyBlocked: []string{"dependency:approval-ledger"}})
				result := runFixtureWithObservation(t, root, digest, proposalPath, authorityPath, observationPath)
				assertUnknownTuple(t, result)
				return
			}
			result := runFixture(t, root, digest, proposalPath, authorityPath)
			assertUnknownTuple(t, result)
		})
	}
}

func assertUnknownTuple(t *testing.T, result Result) {
	t.Helper()
	if result.Manifest.Decision != DecisionUnknown || len(result.Manifest.Unknowns) == 0 {
		t.Fatalf("expected UNKNOWN with tuple, got %s %+v", result.Manifest.Decision, result.Manifest)
	}
	for _, tuple := range result.Manifest.Unknowns {
		if tuple.Stage == "" || tuple.Step == "" || tuple.Reason == "" || tuple.UnknownClass == "" || tuple.NextOperation == "" || len(tuple.BlockedBy) == 0 {
			t.Fatalf("incomplete UNKNOWN tuple: %+v", tuple)
		}
	}
}

func testSource(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func testApprovalChain(t *testing.T, root, path, operation string, after []byte, hunks []Hunk) (string, string) {
	t.Helper()
	rootDigest, err := ComputeSourceDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	before := []byte(nil)
	if operation != OperationAdd {
		before, err = os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
	}
	postDigest := DigestBytes(after)
	if operation == OperationDelete {
		postDigest = EmptyFileDigest
	}
	preDigest := EmptyFileDigest
	if operation != OperationAdd {
		preDigest = DigestBytes(before)
	}
	proposalPath := filepath.Join(t.TempDir(), "proposal.json")
	proposal := Proposal{Schema: ProposalSchema, ProposalID: "proposal-1", Status: "APPROVED", SourceTreeDigest: rootDigest, IntentDigest: intentDigestForTest(t), AuthorityReceiptID: "authority-1", ApprovedBy: "human-1", Changes: []ProposalChange{{Path: path, Operation: operation, PreimageDigest: preDigest, PostimageDigest: postDigest, PostimageBase64: base64.StdEncoding.EncodeToString(after), Hunks: hunks}}}
	writeTestJSON(t, proposalPath, proposal)
	proposal.ProposalDigest = fileSelfDigest(proposalPath, "proposal_digest")
	writeTestJSON(t, proposalPath, proposal)
	authorityPath := filepath.Join(t.TempDir(), "authority.json")
	authority := AuthorityReceipt{Schema: AuthoritySchema, ReceiptID: "authority-1", ProposalID: proposal.ProposalID, ProposalDigest: proposal.ProposalDigest, IntentDigest: intentDigestForTest(t), Approved: true, ApprovedBy: proposal.ApprovedBy}
	writeTestJSON(t, authorityPath, authority)
	authority.ReceiptDigest = fileSelfDigest(authorityPath, "receipt_digest")
	writeTestJSON(t, authorityPath, authority)
	return proposalPath, authorityPath
}

func intentDigestForTest(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, "examples", "change-bundle", "change-intent.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	return DigestBytes(data)
}

func runFixture(t *testing.T, root, digest, proposal, authority string) Result {
	t.Helper()
	return runFixtureWithObservation(t, root, digest, proposal, authority, "")
}

func runFixtureWithObservation(t *testing.T, root, digest, proposal, authority, observationPath string) Result {
	t.Helper()
	repoRoot := filepath.Join("..", "..")
	result, err := Run(Options{SourceRoot: root, SourceDigest: digest, ProposalPath: proposal, AuthorityPath: authority, IntentPath: filepath.Join(repoRoot, "examples", "change-bundle", "change-intent.gooo"), ContractPath: filepath.Join(repoRoot, "contracts", "change-bundle-denominator-v1.json"), OutputDir: filepath.Join(t.TempDir(), "out"), ObservationPath: observationPath})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mutateProposalRollback(t *testing.T, path, value string) {
	t.Helper()
	var proposal Proposal
	if err := readJSON(path, &proposal); err != nil {
		t.Fatal(err)
	}
	proposal.Changes[0].RollbackPostimageDigest = value
	proposal.ProposalDigest = ""
	writeTestJSON(t, path, proposal)
	proposal.ProposalDigest = fileSelfDigest(path, "proposal_digest")
	writeTestJSON(t, path, proposal)
}

func mutateProposalStatus(t *testing.T, path, status string) {
	t.Helper()
	var proposal Proposal
	if err := readJSON(path, &proposal); err != nil {
		t.Fatal(err)
	}
	proposal.Status = status
	proposal.ProposalDigest = ""
	writeTestJSON(t, path, proposal)
	proposal.ProposalDigest = fileSelfDigest(path, "proposal_digest")
	writeTestJSON(t, path, proposal)
}
