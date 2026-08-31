package bundle

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type observation struct {
	SourceTreeObservable bool     `json:"source_tree_observable"`
	DirectMissing        []string `json:"direct_missing"`
	DependencyBlocked    []string `json:"dependency_blocked"`
}

type patchOperation struct {
	Path            string `json:"path"`
	Operation       string `json:"operation"`
	PreimageDigest  string `json:"preimage_digest"`
	PostimageDigest string `json:"postimage_digest"`
	PreimageBase64  string `json:"preimage_base64"`
	PostimageBase64 string `json:"postimage_base64"`
}

type patchDocument struct {
	Schema     string           `json:"schema"`
	Direction  string           `json:"direction"`
	Decision   string           `json:"decision"`
	Operations []patchOperation `json:"operations"`
}

type changeSummary struct {
	Decision              string        `json:"decision"`
	SourceTreeDigest      string        `json:"source_tree_digest"`
	PreimageTreeDigest    string        `json:"preimage_tree_digest"`
	PostimageTreeDigest   string        `json:"postimage_tree_digest"`
	ChangedPaths          []string      `json:"changed_paths"`
	Targets               []TargetEntry `json:"targets"`
	Findings              []Finding     `json:"findings"`
	ApplyAuthorized       bool          `json:"apply_authorized"`
	CommitAuthorized      bool          `json:"commit_authorized"`
	PushAuthorized        bool          `json:"push_authorized"`
	PullRequestAuthorized bool          `json:"pull_request_authorized"`
	MergeAuthorized       bool          `json:"merge_authorized"`
}

type safetyInputs struct {
	Intent          Intent
	Proposal        Proposal
	Authority       AuthorityReceipt
	ProposalDigest  string
	AuthorityDigest string
	Snapshot        treeSnapshot
	TreeObserved    bool
}

func Run(options Options) (Result, error) {
	if options.OutputDir == "" {
		return Result{}, fmt.Errorf("output directory is required")
	}
	intent, err := ParseIntent(options.IntentPath)
	if err != nil {
		return Result{}, err
	}
	contract, err := LoadContract(options.ContractPath)
	if err != nil {
		return Result{}, err
	}
	if err := ValidateDeclarations(intent, contract); err != nil {
		return Result{}, err
	}
	if err := ensureOutputBoundary(options.SourceRoot, options.OutputDir); err != nil {
		return Result{}, err
	}

	proposal, proposalErr := LoadProposal(options.ProposalPath)
	authority, authorityErr := LoadAuthorityReceipt(options.AuthorityPath)
	proposalDigest := fileSelfDigest(options.ProposalPath, "proposal_digest")
	authorityDigest := fileSelfDigest(options.AuthorityPath, "receipt_digest")

	snapshot, treeErr := collectTree(options.SourceRoot)
	treeObserved := treeErr == nil
	if !treeObserved {
		snapshot = treeSnapshot{Manifest: TreeManifest{Schema: TreeManifestSchema, RootPolicy: "NO_SYMLINKS; EXCLUDE_EXACT_ROOT_.git"}, Files: map[string][]byte{}}
	}
	findings := make([]Finding, 0, 8)
	if options.SourceDigest == "" {
		findings = append(findings, unknownFinding("SOURCE_DIGEST_MISSING", "SOURCE_BINDING", "REQUIRE_EXACT_SOURCE_TREE_DIGEST", "direct_missing", "PROVIDE_SOURCE_TREE_DIGEST", []string{"source_tree_digest"}, "exact source tree digest was not provided"))
	}
	if treeErr != nil {
		var observedErr *treeError
		if errors.As(treeErr, &observedErr) && observedErr.Code == "SYMLINK_ESCAPE" {
			findings = append(findings, refutedFinding("SYMLINK_ESCAPE", observedErr.Path, "source tree contains a symlink or symlink root"))
		} else {
			path := ""
			if observedErr != nil {
				path = observedErr.Path
			}
			findings = append(findings, unknownFinding("SOURCE_TREE_UNOBSERVABLE", "SOURCE_OBSERVATION", "OBSERVE_SOURCE_TREE", "observation_unavailable", "MAKE_SOURCE_TREE_READABLE", []string{"source_tree"}, "source tree could not be observed"))
			if path != "" {
				findings[len(findings)-1].Path = path
			}
		}
	}
	if treeObserved && options.SourceDigest != "" && options.SourceDigest != snapshot.Manifest.SourceDigest {
		findings = append(findings, refutedFinding("STALE_PREIMAGE", "", "provided source tree digest does not match the observed source tree"))
	}
	if options.SourceDigest != "" && proposal.SourceTreeDigest != "" && proposal.SourceTreeDigest != options.SourceDigest {
		findings = append(findings, refutedFinding("PROPOSAL_SOURCE_BINDING_MISMATCH", "", "approved proposal is bound to a different source tree digest"))
	}
	if proposalErr != nil {
		if os.IsNotExist(proposalErr) {
			findings = append(findings, unknownFinding("PROPOSAL_DIRECT_MISSING", "PROPOSAL_INPUT", "READ_APPROVED_PROPOSAL", "direct_missing", "PROVIDE_APPROVED_PROPOSAL", []string{"proposal"}, "approved proposal was not observable"))
		} else {
			findings = append(findings, refutedFinding("PROPOSAL_MALFORMED", "", proposalErr.Error()))
		}
	}
	if authorityErr != nil {
		if os.IsNotExist(authorityErr) {
			findings = append(findings, unknownFinding("AUTHORITY_DIRECT_MISSING", "AUTHORITY_INPUT", "READ_AUTHORITY_RECEIPT", "direct_missing", "PROVIDE_AUTHORITY_RECEIPT", []string{"authority_receipt"}, "authority receipt was not observable"))
		} else {
			findings = append(findings, refutedFinding("AUTHORITY_MALFORMED", "", authorityErr.Error()))
		}
	}

	inputs := safetyInputs{Intent: intent, Proposal: proposal, Authority: authority, ProposalDigest: proposalDigest, AuthorityDigest: authorityDigest, Snapshot: snapshot, TreeObserved: treeObserved}
	findings = append(findings, validateAuthorityAndIdentity(options, inputs)...)
	findings = append(findings, validateObservation(options.ObservationPath)...)
	targets, sortedChanges, changeFindings := validateChanges(inputs)
	findings = append(findings, changeFindings...)

	decision := resolve(findings)
	post := snapshot
	var patchOps []patchOperation
	patchRoundTrip := false
	rollbackRoundTrip := false
	if decision == DecisionClosed {
		post, err = postimageSnapshot(snapshot, sortedChanges)
		if err != nil {
			findings = append(findings, refutedFinding("POSTIMAGE_CONSTRUCTION_FAILED", "", err.Error()))
			decision = resolve(findings)
		} else {
			patchOps = makePatchOperations(snapshot.Files, sortedChanges)
			forward, forwardErr := applyPatch(snapshot.Files, sortedChanges)
			backward, backwardErr := applyRollback(post.Files, snapshot.Files, sortedChanges)
			patchRoundTrip = forwardErr == nil && stateDigest(forward) == stateDigest(post.Files)
			rollbackRoundTrip = backwardErr == nil && stateDigest(backward) == stateDigest(snapshot.Files)
			if !patchRoundTrip {
				findings = append(findings, refutedFinding("PATCH_ROUND_TRIP_MISMATCH", "", "forward patch did not produce the expected postimage"))
			}
			if !rollbackRoundTrip {
				findings = append(findings, refutedFinding("ROLLBACK_MISMATCH", "", "rollback did not restore the exact preimage"))
			}
			decision = resolve(findings)
		}
	}
	if decision != DecisionClosed {
		patchOps = nil
		post = snapshot
	}

	unknowns := make([]UnknownTuple, 0)
	for _, finding := range findings {
		if finding.Unknown != nil {
			unknowns = append(unknowns, *finding.Unknown)
		}
	}
	core, err := buildArtifacts(intent, proposal, authority, snapshot, post, decision, findings, unknowns, targets, patchOps, patchRoundTrip, rollbackRoundTrip)
	if err != nil {
		return Result{}, err
	}
	replayed, err := buildArtifacts(intent, proposal, authority, snapshot, post, decision, findings, unknowns, targets, patchOps, patchRoundTrip, rollbackRoundTrip)
	if err != nil {
		return Result{}, err
	}
	replay := replayReceipt(core, replayed, decision, patchRoundTrip, rollbackRoundTrip)
	replayBytes, err := jsonBytes(replay)
	if err != nil {
		return Result{}, err
	}
	core["replay-receipt.json"] = replayBytes
	manifest, err := buildManifest(intent, proposal, authority, snapshot, post, decision, findings, unknowns, core, replay, targets)
	if err != nil {
		return Result{}, err
	}
	manifestBytes, err := jsonBytes(manifest)
	if err != nil {
		return Result{}, err
	}
	if err := writeArtifacts(options.OutputDir, core, manifestBytes); err != nil {
		return Result{}, err
	}
	core["bundle-manifest.json"] = manifestBytes
	return Result{Manifest: manifest, Files: core}, nil
}

func ComputeSourceDigest(root string) (string, error) {
	snapshot, err := collectTree(root)
	if err != nil {
		return "", err
	}
	return snapshot.Manifest.SourceDigest, nil
}

func ensureOutputBoundary(sourceRoot, outputDir string) error {
	root, err := absolutePath(sourceRoot)
	if err != nil {
		return err
	}
	out, err := absolutePath(outputDir)
	if err != nil {
		return err
	}
	if out == root || pathWithin(root, out) {
		return fmt.Errorf("output must be outside the input source tree")
	}
	if info, statErr := os.Lstat(out); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("output directory must not be a symlink")
		}
		if !info.IsDir() {
			return fmt.Errorf("output path is not a directory")
		}
		entries, readErr := os.ReadDir(out)
		if readErr != nil {
			return readErr
		}
		if len(entries) != 0 {
			return fmt.Errorf("output directory must be empty")
		}
		return nil
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	return os.MkdirAll(out, 0o755)
}

func fileSelfDigest(path, field string) string {
	if path == "" {
		return ""
	}
	digest, err := digestWithoutField(path, field)
	if err != nil {
		return ""
	}
	return digest
}

func validateAuthorityAndIdentity(options Options, inputs safetyInputs) []Finding {
	findings := make([]Finding, 0, 8)
	proposal := inputs.Proposal
	authority := inputs.Authority
	if inputs.ProposalDigest == "" || proposal.ProposalDigest == "" || inputs.ProposalDigest != proposal.ProposalDigest {
		findings = append(findings, refutedFinding("PROPOSAL_DIGEST_UNBOUND", "", "proposal digest is absent or does not bind the exact proposal bytes"))
	}
	if inputs.AuthorityDigest == "" || authority.ReceiptDigest == "" || inputs.AuthorityDigest != authority.ReceiptDigest {
		findings = append(findings, refutedFinding("AUTHORITY_DIGEST_UNBOUND", "", "authority receipt digest is absent or does not bind the exact receipt bytes"))
	}
	if proposal.Status != "APPROVED" || proposal.ProposalID == "" || proposal.ApprovedBy == "" {
		findings = append(findings, refutedFinding("UNAUTHORIZED_PROPOSAL", "", "proposal is not explicitly approved by a named authority"))
	}
	if !authority.Approved || authority.ProposalID == "" || authority.ApprovedBy == "" || authority.ApprovedBy != proposal.ApprovedBy || authority.ProposalID != proposal.ProposalID {
		findings = append(findings, refutedFinding("UNAUTHORIZED_AUTHORITY_RECEIPT", "", "authority receipt does not approve the exact proposal"))
	}
	if authority.ProposalDigest != proposal.ProposalDigest ||
		(proposal.AuthorityReceiptID != "" && proposal.AuthorityReceiptID != authority.ReceiptID) ||
		proposal.IntentDigest != inputs.Intent.SourceDigest || authority.IntentDigest != inputs.Intent.SourceDigest {
		findings = append(findings, refutedFinding("AUTHORITY_CHAIN_MISMATCH", "", "proposal, authority receipt, and intent are not bound to one exact chain"))
	}
	if !authority.Authority().IsZero() {
		findings = append(findings, refutedFinding("AUTHORITY_ESCALATION", "", "apply, commit, push, pull request, merge, and repository-write authority must all be zero"))
	}
	if options.SourceDigest != "" && proposal.SourceTreeDigest != options.SourceDigest {
		findings = append(findings, refutedFinding("STALE_PREIMAGE", "", "proposal source digest is not the supplied exact source digest"))
	}
	return findings
}

func validateObservation(path string) []Finding {
	if path == "" {
		return nil
	}
	var value observation
	if err := readJSON(path, &value); err != nil {
		if os.IsNotExist(err) {
			return []Finding{unknownFinding("OBSERVATION_DIRECT_MISSING", "OBSERVATION", "READ_OPTIONAL_OBSERVATION", "direct_missing", "PROVIDE_OBSERVATION", []string{"observation"}, "optional observation was not observable")}
		}
		return []Finding{refutedFinding("OBSERVATION_MALFORMED", "", err.Error())}
	}
	findings := make([]Finding, 0, len(value.DirectMissing)+len(value.DependencyBlocked)+1)
	for _, item := range value.DirectMissing {
		findings = append(findings, unknownFinding("OBSERVATION_DIRECT_MISSING", "OBSERVATION", "OBSERVE_DIRECT_INPUT", "direct_missing", "PROVIDE_MISSING_INPUT", []string{item}, "direct observation is missing"))
	}
	for _, item := range value.DependencyBlocked {
		findings = append(findings, unknownFinding("OBSERVATION_DEPENDENCY_BLOCKED", "OBSERVATION", "RESOLVE_DEPENDENCY", "dependency_blocked", "RESOLVE_BLOCKING_DEPENDENCY", []string{item}, "observation is blocked by a dependency"))
	}
	if !value.SourceTreeObservable {
		findings = append(findings, unknownFinding("OBSERVATION_UNAVAILABLE", "OBSERVATION", "OBSERVE_SOURCE_TREE", "observation_unavailable", "MAKE_SOURCE_TREE_OBSERVABLE", []string{"source_tree"}, "source tree observation was declared unavailable"))
	}
	return findings
}

func validateChanges(inputs safetyInputs) ([]TargetEntry, []ProposalChange, []Finding) {
	proposal := inputs.Proposal
	changes := append([]ProposalChange(nil), proposal.Changes...)
	findings := make([]Finding, 0, len(changes))
	seen := make(map[string]bool, len(changes))
	for _, change := range changes {
		if !validRelativePath(change.Path) {
			findings = append(findings, refutedFinding("PATH_TRAVERSAL", change.Path, "target path is absolute, non-canonical, or escapes the source root"))
			continue
		}
		if seen[change.Path] {
			findings = append(findings, refutedFinding("CONFLICTING_HUNKS", change.Path, "more than one hunk claims the same target path"))
			continue
		}
		seen[change.Path] = true
		if isGeneratedPath(change.Path) {
			findings = append(findings, refutedFinding("GENERATED_FILE_AUTHORITY", change.Path, "generated files require a separate generated-file authority and cannot be changed by this bundle"))
		}
		if overlap(change.Hunks) {
			findings = append(findings, refutedFinding("CONFLICTING_HUNKS", change.Path, "hunk ranges overlap"))
		}
		if change.Operation != OperationAdd && change.Operation != OperationModify && change.Operation != OperationDelete {
			findings = append(findings, refutedFinding("UNSUPPORTED_OPERATION", change.Path, "change operation is not ADD, MODIFY, or DELETE"))
			continue
		}
		if change.PreimageDigest == "" {
			findings = append(findings, refutedFinding("PREIMAGE_DIGEST_MISSING", change.Path, "every target requires an exact preimage digest"))
		}
		if change.Operation == OperationAdd && change.PreimageDigest != EmptyFileDigest {
			findings = append(findings, refutedFinding("ADD_PREIMAGE_NOT_EMPTY", change.Path, "ADD requires the empty-file sentinel preimage"))
		}
		if change.Operation == OperationDelete && change.PostimageDigest != EmptyFileDigest {
			findings = append(findings, refutedFinding("DELETE_POSTIMAGE_NOT_EMPTY", change.Path, "DELETE requires the empty-file sentinel postimage"))
		}
		if _, err := decodePostimage(change); err != nil {
			findings = append(findings, refutedFinding("POSTIMAGE_DIGEST_MISMATCH", change.Path, err.Error()))
		}
		if change.RollbackPostimageDigest != "" && change.RollbackPostimageDigest != change.PreimageDigest {
			findings = append(findings, refutedFinding("ROLLBACK_MISMATCH", change.Path, "declared rollback postimage does not equal the exact preimage"))
		}
		if inputs.TreeObserved {
			data, exists := inputs.Snapshot.Files[change.Path]
			actual := EmptyFileDigest
			if exists {
				actual = DigestBytes(data)
			}
			switch change.Operation {
			case OperationAdd:
				if exists {
					findings = append(findings, refutedFinding("TARGET_ALREADY_EXISTS", change.Path, "ADD target already exists in the exact source tree"))
				}
			case OperationModify, OperationDelete:
				if !exists {
					findings = append(findings, refutedFinding("PREIMAGE_MISSING", change.Path, "MODIFY or DELETE target is absent from the exact source tree"))
				}
			}
			if change.PreimageDigest != actual {
				findings = append(findings, refutedFinding("STALE_PREIMAGE", change.Path, "target preimage digest does not match the exact source file"))
			}
			if parentErr := targetParentError(inputs.Snapshot, change.Path); parentErr != "" {
				findings = append(findings, refutedFinding("SYMLINK_ESCAPE", change.Path, parentErr))
			}
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	targets := make([]TargetEntry, 0, len(changes))
	for _, change := range changes {
		hunkCount := len(change.Hunks)
		if hunkCount == 0 {
			hunkCount = 1
		}
		targets = append(targets, TargetEntry{Path: change.Path, Operation: change.Operation, PreimageDigest: change.PreimageDigest, PostimageDigest: change.PostimageDigest, HunkCount: thunkCount})
	}
	return targets, changes, findings
}

func targetParentError(snapshot treeSnapshot, path string) string {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	for index := 1; index < len(parts); index++ {
		candidate := strings.Join(parts[:index], "/")
		for _, entry := range snapshot.Manifest.Entries {
			if entry.Path == candidate && entry.Kind != "directory" {
				return "target parent is not a directory"
			}
		}
	}
	return ""
}

func isGeneratedPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	base := filepath.Base(clean)
	return clean == "generated" || strings.HasPrefix(clean, "generated/") || strings.Contains(clean, "/generated/") || strings.HasSuffix(base, ".gen.go") || strings.Contains(base, ".generated.")
}

func overlap(hunks []Hunk) bool {
	for _, hunk := range hunks {
		if hunk.StartLine < 1 || hunk.EndLine < hunk.StartLine {
			return true
		}
	}
	copyHunks := append([]Hunk(nil), hunks...)
	sort.Slice(copyHunks, func(i, j int) bool { return copyHunks[i].StartLine < copyHunks[j].StartLine })
	for index := 1; index < len(copyHunks); index++ {
		if copyHunks[index].StartLine <= copyHunks[index-1].EndLine {
			return true
		}
	}
	return false
}

func decodePostimage(change ProposalChange) ([]byte, error) {
	if change.Operation == OperationDelete {
		if change.PostimageDigest != EmptyFileDigest {
			return nil, fmt.Errorf("DELETE postimage must be the empty-file sentinel")
		}
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(change.PostimageBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid postimage base64: %w", err)
	}
	if change.PostimageDigest == "" || DigestBytes(data) != change.PostimageDigest {
		return nil, fmt.Errorf("postimage bytes do not match the declared digest")
	}
	return data, nil
}

func makePatchOperations(files map[string][]byte, changes []ProposalChange) []patchOperation {
	operations := make([]patchOperation, 0, len(changes))
	for _, change := range changes {
		before := files[change.Path]
		after, _ := decodePostimage(change)
		operations = append(operations, patchOperation{Path: change.Path, Operation: change.Operation, PreimageDigest: change.PreimageDigest, PostimageDigest: change.PostimageDigest, PreimageBase64: base64.StdEncoding.EncodeToString(before), PostimageBase64: base64.StdEncoding.EncodeToString(after)})
	}
	return operations
}

func applyPatch(files map[string][]byte, changes []ProposalChange) (map[string][]byte, error) {
	result := make(map[string][]byte, len(files))
	for path, data := range files {
		result[path] = append([]byte(nil), data...)
	}
	for _, change := range changes {
		current, exists := result[change.Path]
		actual := EmptyFileDigest
		if exists {
			actual = DigestBytes(current)
		}
		if actual != change.PreimageDigest {
			return nil, fmt.Errorf("forward preimage mismatch for %s", change.Path)
		}
		data, err := decodePostimage(change)
		if err != nil {
			return nil, err
		}
		if change.Operation == OperationDelete {
			delete(result, change.Path)
		} else {
			result[change.Path] = append([]byte(nil), data...)
		}
	}
	return result, nil
}

func applyRollback(postFiles, preFiles map[string][]byte, changes []ProposalChange) (map[string][]byte, error) {
	result := make(map[string][]byte, len(postFiles))
	for path, data := range postFiles {
		result[path] = append([]byte(nil), data...)
	}
	for _, change := range changes {
		current, exists := result[change.Path]
		actual := EmptyFileDigest
		if exists {
			actual = DigestBytes(current)
		}
		if actual != change.PostimageDigest {
			return nil, fmt.Errorf("rollback postimage mismatch for %s", change.Path)
		}
		before, exists := preFiles[change.Path]
		if change.Operation == OperationAdd {
			delete(result, change.Path)
		} else if exists {
			result[change.Path] = append([]byte(nil), before...)
		} else {
			return nil, fmt.Errorf("rollback preimage missing for %s", change.Path)
		}
	}
	return result, nil
}

func stateDigest(files map[string][]byte) string {
	paths := sortedKeys(files)
	var builder strings.Builder
	for _, path := range paths {
		builder.WriteString(path)
		builder.WriteByte(0)
		builder.Write(files[path])
		builder.WriteByte(0)
	}
	return DigestBytes([]byte(builder.String()))
}

func resolve(findings []Finding) string {
	for _, finding := range findings {
		if finding.State == DecisionRefuted {
			return DecisionRefuted
		}
	}
	for _, finding := range findings {
		if finding.State == DecisionUnknown {
			return DecisionUnknown
		}
	}
	return DecisionClosed
}

func refutedFinding(code, path, reason string) Finding {
	return Finding{Code: code, State: DecisionRefuted, Path: path, Reason: reason}
}

func unknownFinding(code, stage, step, class, next string, blocked []string, reason string) Finding {
	return Finding{Code: code, State: DecisionUnknown, Reason: reason, Unknown: &UnknownTuple{Stage: stage, Step: step, Reason: reason, UnknownClass: class, NextOperation: next, BlockedBy: append([]string(nil), blocked...)}}
}

func buildArtifacts(intent Intent, proposal Proposal, authority AuthorityReceipt, snapshot, post treeSnapshot, decision string, findings []Finding, unknowns []UnknownTuple, targets []TargetEntry, patchOps []patchOperation, patchRoundTrip, rollbackRoundTrip bool) (map[string][]byte, error) {
	files := make(map[string][]byte)
	if data, err := jsonBytes(snapshot.Manifest); err != nil {
		return nil, err
	} else {
		files["source-tree-manifest.json"] = data
	}
	if data, err := jsonBytes(targets); err != nil {
		return nil, err
	} else {
		files["target-manifest.json"] = data
	}
	preimages := make([]map[string]string, 0, len(patchOps))
	postimages := make([]map[string]string, 0, len(patchOps))
	for _, operation := range patchOps {
		preimages = append(preimages, map[string]string{"path": operation.Path, "digest": operation.PreimageDigest})
		postimages = append(postimages, map[string]string{"path": operation.Path, "digest": operation.PostimageDigest})
	}
	if data, err := jsonBytes(preimages); err != nil {
		return nil, err
	} else {
		files["preimage-digests.json"] = data
	}
	if data, err := jsonBytes(postimages); err != nil {
		return nil, err
	} else {
		files["postimage-digests.json"] = data
	}
	patch := patchDocument{Schema: "gooo/change-bundle/patch/v1", Direction: "FORWARD", Decision: decision, Operations: patchOps}
	rollbackOps := make([]patchOperation, 0, len(patchOps))
	for _, operation := range patchOps {
		rollbackOps = append(rollbackOps, patchOperation{Path: operation.Path, Operation: operation.Operation, PreimageDigest: operation.PostimageDigest, PostimageDigest: operation.PreimageDigest, PreimageBase64: operation.PostimageBase64, PostimageBase64: operation.PreimageBase64})
	}
	rollback := patchDocument{Schema: "gooo/change-bundle/patch/v1", Direction: "ROLLBACK", Decision: decision, Operations: rollbackOps}
	if data, err := jsonBytes(patch); err != nil {
		return nil, err
	} else {
		files["patch.bundle.json"] = data
	}
	if data, err := jsonBytes(rollback); err != nil {
		return nil, err
	} else {
		files["rollback.bundle.json"] = data
	}
	files["patch.diff"] = []byte(renderPatchText(patch))
	files["rollback.diff"] = []byte(renderPatchText(rollback))
	ir := IR{Schema: IRSchema, Version: "v1", DenominatorID: intent.DenominatorID, CellCount: intent.CellCount, SourceDigest: optionsDigest(snapshot), IntentDigest: intent.SourceDigest, ProposalDigest: proposal.ProposalDigest, AuthorityReceiptDigest: authority.ReceiptDigest, Precedence: intent.Precedence, UnknownFields: intent.UnknownFields, Activities: intent.Activities, Authority: Authority{}}
	if data, err := jsonBytes(ir); err != nil {
		return nil, err
	} else {
		files["semantic-ir.json"] = data
	}
	files["generated/evaluator.go"] = []byte(renderEvaluator(intent))
	precondition := PreconditionReceipt{Schema: PreconditionSchema, Decision: decision, SourceTreeDigest: snapshot.Manifest.SourceDigest, ProposalDigest: proposal.ProposalDigest, AuthorityReceiptDigest: authority.ReceiptDigest, IntentDigest: intent.SourceDigest, Targets: targets, InputRepositoryWrites: 0, ApplyAuthorized: false, CommitAuthorized: false, PushAuthorized: false, PullRequestAuthorized: false, MergeAuthorized: false}
	if data, err := jsonBytes(precondition); err != nil {
		return nil, err
	} else {
		files["apply-precondition-receipt.json"] = data
	}
	summary := changeSummary{Decision: decision, SourceTreeDigest: snapshot.Manifest.SourceDigest, PreimageTreeDigest: snapshot.Manifest.SourceDigest, PostimageTreeDigest: post.Manifest.SourceDigest, ChangedPaths: targetPaths(targets), Targets: targets, Findings: findings, ApplyAuthorized: false, CommitAuthorized: false, PushAuthorized: false, PullRequestAuthorized: false, MergeAuthorized: false}
	if data, err := jsonBytes(summary); err != nil {
		return nil, err
	} else {
		files["change-summary.json"] = data
	}
	files["human-dossier.md"] = []byte(renderDossier(intent, proposal, authority, snapshot, post, decision, findings, unknowns, targets, patchRoundTrip, rollbackRoundTrip))
	return files, nil
}

func optionsDigest(snapshot treeSnapshot) string {
	return snapshot.Manifest.SourceDigest
}

func targetPaths(targets []TargetEntry) []string {
	paths := make([]string, 0, len(targets))
	for _, target := range targets {
		paths = append(paths, target.Path)
	}
	return paths
}

func replayReceipt(files, replayed map[string][]byte, decision string, patchRoundTrip, rollbackRoundTrip bool) ReplayReceipt {
	keySet := make(map[string]struct{}, len(files)+len(replayed))
	for key := range files {
		keySet[key] = struct{}{}
	}
	for key := range replayed {
		keySet[key] = struct{}{}
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	comparisons := 0
	mismatches := 0
	for _, key := range keys {
		comparisons++
		if !bytes.Equal(files[key], replayed[key]) {
			mismatches++
		}
	}
	rollbackComparisons := 0
	rollbackMismatches := 0
	if len(keys) > 0 {
		rollbackComparisons = 1
		rollbackMismatches = boolInt(!rollbackRoundTrip)
	}
	return ReplayReceipt{Schema: ReplaySchema, Decision: decision, Comparisons: comparisons, Mismatches: mismatches, ComparedArtifacts: keys, RollbackComparisons: rollbackComparisons, RollbackMismatches: rollbackMismatches, PatchRoundTrip: patchRoundTrip, RollbackRoundTrip: rollbackRoundTrip, InputRepositoryWrites: 0}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func buildManifest(intent Intent, proposal Proposal, authority AuthorityReceipt, snapshot, post treeSnapshot, decision string, findings []Finding, unknowns []UnknownTuple, files map[string][]byte, replay ReplayReceipt, targets []TargetEntry) (BundleManifest, error) {
	artifacts := make([]Artifact, 0, len(files))
	var total int64
	for _, path := range sortedKeys(files) {
		data := files[path]
		artifacts = append(artifacts, Artifact{Path: path, Bytes: int64(len(data)), Digest: DigestBytes(data)})
		total += int64(len(data))
	}
	metrics := Metrics{BundleFileCount: len(files), BundleBytes: total, ChangedPaths: len(targets), ChangedHunks: hunkCount(targets), ReplayComparisons: replay.Comparisons, ReplayMismatches: replay.Mismatches, RollbackComparisons: replay.RollbackComparisons, RollbackMismatches: replay.RollbackMismatches, Files: fileCount(snapshot.Manifest), Directories: directoryCount(snapshot.Manifest), RepositoryWrites: 0, LocalTestExecutions: 0, CrossProjectRequiredGates: 0}
	return BundleManifest{Schema: BundleSchema, Version: "v1", Decision: decision, SourceTreeDigest: snapshot.Manifest.SourceDigest, IntentDigest: intent.SourceDigest, ProposalDigest: proposal.ProposalDigest, AuthorityReceiptDigest: authority.ReceiptDigest, PreimageTreeDigest: snapshot.Manifest.SourceDigest, PostimageTreeDigest: post.Manifest.SourceDigest, ChangedPaths: targetPaths(targets), Findings: findings, Unknowns: unknowns, Artifacts: artifacts, Authority: Authority{}, Metrics: metrics}, nil
}

func writeArtifacts(output string, files map[string][]byte, manifest []byte) error {
	for _, path := range sortedKeys(files) {
		full := filepath.Join(output, filepath.FromSlash(path))
		if !pathWithin(output, full) && full != output {
			return fmt.Errorf("artifact path escapes output: %s", path)
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, files[path], 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(output, "bundle-manifest.json"), manifest, 0o644)
}

func fileCount(manifest TreeManifest) int {
	count := 0
	for _, entry := range manifest.Entries {
		if entry.Kind == "file" {
			count++
		}
	}
	return count
}

func directoryCount(manifest TreeManifest) int {
	count := 0
	for _, entry := range manifest.Entries {
		if entry.Kind == "directory" {
			count++
		}
	}
	return count
}

func hunkCount(targets []TargetEntry) int {
	count := 0
	for _, target := range targets {
		count += target.HunkCount
	}
	return count
}
