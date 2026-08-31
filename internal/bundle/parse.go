package bundle

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func ParseIntent(path string) (Intent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Intent{}, err
	}
	intent := Intent{SourceDigest: DigestBytes(data)}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		line = strings.TrimSpace(strings.SplitN(line, "//", 2)[0])
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		values, valueErr := func() (map[string]string, error) {
			if len(fields) < 2 {
				return nil, fmt.Errorf("line %d: missing declaration fields", lineNumber)
			}
			return keyValues(fields[1:])
		}()
		switch fields[0] {
		case "gooo":
			if len(fields) != 3 || fields[1] != "change_bundle" || fields[2] != "v1" {
				return Intent{}, fmt.Errorf("line %d: invalid gooo header", lineNumber)
			}
			intent.Schema = SourceSchema
			intent.Version = fields[2]
		case "denominator":
			if valueErr != nil {
				return Intent{}, valueErr
			}
			intent.DenominatorID = values["id"]
			intent.CellCount, err = integer(values, "cell_count")
			if err != nil {
				return Intent{}, fmt.Errorf("line %d: %w", lineNumber, err)
			}
		case "authority":
			if valueErr != nil {
				return Intent{}, valueErr
			}
			intent.Authority, err = authorityFromValues(values)
			if err != nil {
				return Intent{}, fmt.Errorf("line %d: %w", lineNumber, err)
			}
		case "precedence":
			if len(fields) != 2 {
				return Intent{}, fmt.Errorf("line %d: invalid precedence", lineNumber)
			}
			intent.Precedence = strings.Split(fields[1], ">")
		case "unknown_fields":
			if len(fields) != 2 {
				return Intent{}, fmt.Errorf("line %d: invalid unknown_fields", lineNumber)
			}
			intent.UnknownFields = strings.Split(fields[1], ",")
		case "activity":
			if valueErr != nil {
				return Intent{}, valueErr
			}
			ordinal, parseErr := integer(values, "ordinal")
			if parseErr != nil {
				return Intent{}, fmt.Errorf("line %d: %w", lineNumber, parseErr)
			}
			intent.Activities = append(intent.Activities, Activity{
				Ordinal: ordinal, ID: values["id"], ProofChoice: values["proof_choice"],
				IndicatorClass: values["indicator_class"], Input: values["input"],
				Output: values["output"], Edge: values["edge"],
			})
		default:
			return Intent{}, fmt.Errorf("line %d: unknown declaration %q", lineNumber, fields[0])
		}
	}
	if err := scanner.Err(); err != nil {
		return Intent{}, err
	}
	return intent, nil
}

func LoadContract(path string) (Contract, error) {
	var contract Contract
	if err := readJSON(path, &contract); err != nil {
		return Contract{}, err
	}
	if contract.Schema != ContractSchema {
		return Contract{}, fmt.Errorf("unexpected contract schema %q", contract.Schema)
	}
	return contract, nil
}

func ValidateDeclarations(intent Intent, contract Contract) error {
	if intent.Schema != SourceSchema || intent.Version != "v1" || intent.DenominatorID != contract.ID ||
		intent.CellCount != FixedCells || contract.Version != "v1" || contract.CellCount != FixedCells || !contract.Fixed {
		return fmt.Errorf("fixed 12-cell denominator declaration mismatch")
	}
	if !sameStrings(intent.Precedence, []string{DecisionRefuted, DecisionUnknown, DecisionClosed}) {
		return fmt.Errorf("resolution precedence mismatch")
	}
	if !sameStrings(intent.UnknownFields, []string{"stage", "step", "reason", "unknown_class", "next_operation", "blocked_by"}) {
		return fmt.Errorf("UNKNOWN six-field contract mismatch")
	}
	if !intent.Authority.IsZero() {
		return fmt.Errorf("intent authority must be zero")
	}
	if len(intent.Activities) != FixedActivities || len(contract.Activities) != FixedActivities {
		return fmt.Errorf("exactly twelve activities are required")
	}
	if contract.ProofBuckets[ProofFoundation] != 4 || contract.ProofBuckets[ProofCoherence] != 4 || contract.ProofBuckets[ProofRegression] != 4 ||
		contract.IndicatorBuckets[IndicatorDriver] != 4 || contract.IndicatorBuckets[IndicatorOutcome] != 4 || contract.IndicatorBuckets[IndicatorGuardrail] != 4 {
		return fmt.Errorf("proof and indicator buckets must be four each")
	}
	for index := range intent.Activities {
		left, right := intent.Activities[index], contract.Activities[index]
		if left.Ordinal != index+1 || right.Ordinal != index+1 || left.ID == "" || left.ID != right.ID ||
			left.ProofChoice != right.ProofChoice || left.IndicatorClass != right.IndicatorClass ||
			left.Input != right.Input || left.Output != right.Output || left.Edge != right.Edge {
			return fmt.Errorf("activity %d is not a one-to-one contract binding", index+1)
		}
	}
	return nil
}

func LoadProposal(path string) (Proposal, error) {
	var proposal Proposal
	if err := readJSON(path, &proposal); err != nil {
		return Proposal{}, err
	}
	if proposal.Schema != ProposalSchema {
		return Proposal{}, fmt.Errorf("unexpected proposal schema %q", proposal.Schema)
	}
	return proposal, nil
}

func LoadAuthorityReceipt(path string) (AuthorityReceipt, error) {
	var receipt AuthorityReceipt
	if err := readJSON(path, &receipt); err != nil {
		return AuthorityReceipt{}, err
	}
	if receipt.Schema != AuthoritySchema {
		return AuthorityReceipt{}, fmt.Errorf("unexpected authority receipt schema %q", receipt.Schema)
	}
	return receipt, nil
}

func keyValues(fields []string) (map[string]string, error) {
	values := make(map[string]string, len(fields))
	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid key/value %q", field)
		}
		values[parts[0]] = strings.Trim(parts[1], "\"")
	}
	return values, nil
}

func integer(values map[string]string, key string) (int, error) {
	value, ok := values[key]
	if !ok {
		return 0, fmt.Errorf("missing %s", key)
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q", key, value)
	}
	return number, nil
}

func boolean(values map[string]string, key string) (bool, error) {
	value, ok := values[key]
	if !ok {
		return false, fmt.Errorf("missing %s", key)
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid %s %q", key, value)
	}
	return parsed, nil
}

func authorityFromValues(values map[string]string) (Authority, error) {
	repositoryWrites, err := integer(values, "repository_writes")
	if err != nil {
		return Authority{}, err
	}
	localTests, err := integer(values, "local_test_executions")
	if err != nil {
		return Authority{}, err
	}
	crossProject, err := integer(values, "cross_project_required_gates")
	if err != nil {
		return Authority{}, err
	}
	apply, err := boolean(values, "apply_authorized")
	if err != nil {
		return Authority{}, err
	}
	commit, err := boolean(values, "commit_authorized")
	if err != nil {
		return Authority{}, err
	}
	push, err := boolean(values, "push_authorized")
	if err != nil {
		return Authority{}, err
	}
	pullRequest, err := boolean(values, "pull_request_authorized")
	if err != nil {
		return Authority{}, err
	}
	merge, err := boolean(values, "merge_authorized")
	if err != nil {
		return Authority{}, err
	}
	return Authority{RepositoryWrites: repositoryWrites, LocalTestExecutions: localTests,
		CrossProjectRequiredGates: crossProject, ApplyAuthorized: apply, CommitAuthorized: commit,
		PushAuthorized: push, PullRequestAuthorized: pullRequest, MergeAuthorized: merge}, nil
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validateJSONFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var value any
	return json.Unmarshal(data, &value)
}
