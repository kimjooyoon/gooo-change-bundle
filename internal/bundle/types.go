package bundle

const (
	SourceSchema        = "gooo/change-bundle/source/v1"
	ContractSchema      = "gooo/change-bundle/denominator/v1"
	ProposalSchema      = "gooo/change-bundle/approved-proposal/v1"
	AuthoritySchema     = "gooo/change-bundle/authority-receipt/v1"
	TreeManifestSchema  = "gooo/change-bundle/tree-manifest/v1"
	IRSchema            = "gooo/change-bundle/semantic-ir/v1"
	BundleSchema        = "gooo/change-bundle/manifest/v1"
	PreconditionSchema  = "gooo/change-bundle/apply-precondition/v1"
	ReplaySchema        = "gooo/change-bundle/replay/v1"
	MetricsSchema       = "gooo/change-bundle/metrics/v1"
	FixedCells          = 12
	FixedActivities     = 12
	EmptyFileDigest     = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	DecisionClosed      = "CLOSED"
	DecisionUnknown     = "UNKNOWN"
	DecisionRefuted     = "REFUTED"
	OperationAdd        = "ADD"
	OperationModify     = "MODIFY"
	OperationDelete     = "DELETE"
	ProofFoundation     = "FOUNDATION"
	ProofCoherence      = "COHERENCE"
	ProofRegression     = "REGRESSION"
	IndicatorDriver     = "DRIVER"
	IndicatorOutcome    = "OUTCOME"
	IndicatorGuardrail  = "GUARDRAIL"
)

type Authority struct {
	RepositoryWrites          int  `json:"repository_writes"`
	LocalTestExecutions       int  `json:"local_test_executions"`
	CrossProjectRequiredGates int  `json:"cross_project_required_gates"`
	ApplyAuthorized           bool `json:"apply_authorized"`
	CommitAuthorized          bool `json:"commit_authorized"`
	PushAuthorized            bool `json:"push_authorized"`
	PullRequestAuthorized     bool `json:"pull_request_authorized"`
	MergeAuthorized           bool `json:"merge_authorized"`
}

func (a Authority) IsZero() bool {
	return a == (Authority{})
}

type Activity struct {
	Ordinal        int    `json:"ordinal"`
	ID             string `json:"id"`
	ProofChoice    string `json:"proof_choice"`
	IndicatorClass string `json:"indicator_class"`
	Input          string `json:"input"`
	Output         string `json:"output"`
	Edge           string `json:"edge"`
}

type Intent struct {
	Schema        string     `json:"schema"`
	Version       string     `json:"version"`
	DenominatorID string     `json:"denominator_id"`
	CellCount     int        `json:"cell_count"`
	Precedence    []string   `json:"precedence"`
	UnknownFields []string   `json:"unknown_fields"`
	Authority     Authority  `json:"authority"`
	Activities    []Activity `json:"activities"`
	SourceDigest  string     `json:"source_digest"`
}

type Contract struct {
	Schema        string     `json:"schema"`
	ID            string     `json:"id"`
	Version       string     `json:"version"`
	CellCount     int        `json:"cell_count"`
	Fixed         bool       `json:"fixed"`
	ProofBuckets  map[string]int `json:"proof_buckets"`
	IndicatorBuckets map[string]int `json:"indicator_buckets"`
	Activities    []Activity `json:"activities"`
}

type Hunk struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

type ProposalChange struct {
	Path                    string `json:"path"`
	Operation               string `json:"operation"`
	PreimageDigest          string `json:"preimage_digest"`
	PostimageDigest         string `json:"postimage_digest"`
	PostimageBase64         string `json:"postimage_base64"`
	RollbackPostimageDigest string `json:"rollback_postimage_digest,omitempty"`
	Hunks                   []Hunk `json:"hunks,omitempty"`
}

type Proposal struct {
	Schema                 string           `json:"schema"`
	ProposalID             string           `json:"proposal_id"`
	Status                 string           `json:"status"`
	SourceTreeDigest       string           `json:"source_tree_digest"`
	IntentDigest           string           `json:"intent_digest"`
	AuthorityReceiptID     string           `json:"authority_receipt_id"`
	AuthorityReceiptDigest string           `json:"authority_receipt_digest"`
	ApprovedBy             string           `json:"approved_by"`
	Changes                []ProposalChange `json:"changes"`
	ProposalDigest         string           `json:"proposal_digest"`
}

type AuthorityReceipt struct {
	Schema          string `json:"schema"`
	ReceiptID       string `json:"receipt_id"`
	ProposalID      string `json:"proposal_id"`
	ProposalDigest  string `json:"proposal_digest"`
	IntentDigest    string `json:"intent_digest"`
	Approved        bool   `json:"approved"`
	ApprovedBy      string `json:"approved_by"`
	RepositoryWrites          int  `json:"repository_writes"`
	LocalTestExecutions       int  `json:"local_test_executions"`
	CrossProjectRequiredGates int  `json:"cross_project_required_gates"`
	ApplyAuthorized           bool `json:"apply_authorized"`
	CommitAuthorized          bool `json:"commit_authorized"`
	PushAuthorized            bool `json:"push_authorized"`
	PullRequestAuthorized     bool `json:"pull_request_authorized"`
	MergeAuthorized           bool `json:"merge_authorized"`
	ReceiptDigest             string `json:"receipt_digest"`
}

func (r AuthorityReceipt) Authority() Authority {
	return Authority{
		RepositoryWrites: r.RepositoryWrites, LocalTestExecutions: r.LocalTestExecutions,
		CrossProjectRequiredGates: r.CrossProjectRequiredGates, ApplyAuthorized: r.ApplyAuthorized,
		CommitAuthorized: r.CommitAuthorized, PushAuthorized: r.PushAuthorized,
		PullRequestAuthorized: r.PullRequestAuthorized, MergeAuthorized: r.MergeAuthorized,
	}
}

type TreeEntry struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Bytes  int    `json:"bytes"`
	Digest string `json:"digest,omitempty"`
}

type TreeManifest struct {
	Schema        string      `json:"schema"`
	RootPolicy    string      `json:"root_policy"`
	Entries       []TreeEntry `json:"entries"`
	SourceDigest  string      `json:"source_digest"`
}

type TargetEntry struct {
	Path            string `json:"path"`
	Operation       string `json:"operation"`
	PreimageDigest  string `json:"preimage_digest"`
	PostimageDigest string `json:"postimage_digest"`
	HunkCount       int    `json:"hunk_count"`
}

type UnknownTuple struct {
	Stage         string   `json:"stage"`
	Step          string   `json:"step"`
	Reason        string   `json:"reason"`
	UnknownClass  string   `json:"unknown_class"`
	NextOperation string   `json:"next_operation"`
	BlockedBy     []string `json:"blocked_by"`
}

type Finding struct {
	Code       string       `json:"code"`
	State      string       `json:"state"`
	Path       string       `json:"path,omitempty"`
	Reason     string       `json:"reason"`
	Unknown   *UnknownTuple `json:"unknown,omitempty"`
}

type Metrics struct {
	BundleFileCount          int   `json:"bundle_file_count"`
	BundleBytes              int64 `json:"bundle_bytes"`
	ChangedPaths             int   `json:"changed_paths"`
	ChangedHunks             int   `json:"changed_hunks"`
	ReplayComparisons        int   `json:"replay_comparisons"`
	ReplayMismatches         int   `json:"replay_mismatches"`
	RollbackComparisons      int   `json:"rollback_comparisons"`
	RollbackMismatches       int   `json:"rollback_mismatches"`
	BuildWallMS              int64 `json:"build_wall_ms"`
	TestWallMS               int64 `json:"test_wall_ms"`
	ConformanceWallMS        int64 `json:"conformance_wall_ms"`
	PeakRSSKiB               int64 `json:"peak_rss_kib"`
	TestsExecuted            int   `json:"tests_executed"`
	TestsReused              int   `json:"tests_reused"`
	TestsSkipped             int   `json:"tests_skipped"`
	TestsNotObserved         int   `json:"tests_not_observed"`
	GoPhysicalLines          int   `json:"go_physical_lines"`
	GoooPhysicalLines        int   `json:"gooo_physical_lines"`
	Files                    int   `json:"files"`
	Directories              int   `json:"directories"`
	RepositoryWrites         int   `json:"repository_writes"`
	LocalTestExecutions      int   `json:"local_test_executions"`
	CrossProjectRequiredGates int  `json:"cross_project_required_gates"`
}

type IR struct {
	Schema                 string     `json:"schema"`
	Version                string     `json:"version"`
	DenominatorID          string     `json:"denominator_id"`
	CellCount              int        `json:"cell_count"`
	SourceDigest           string     `json:"source_digest"`
	IntentDigest           string     `json:"intent_digest"`
	ProposalDigest         string     `json:"proposal_digest"`
	AuthorityReceiptDigest string     `json:"authority_receipt_digest"`
	Precedence             []string   `json:"precedence"`
	UnknownFields          []string   `json:"unknown_fields"`
	Activities             []Activity `json:"activities"`
	Authority              Authority  `json:"authority"`
}

type PreconditionReceipt struct {
	Schema                 string        `json:"schema"`
	Decision               string        `json:"decision"`
	SourceTreeDigest       string        `json:"source_tree_digest"`
	ProposalDigest         string        `json:"proposal_digest"`
	AuthorityReceiptDigest string        `json:"authority_receipt_digest"`
	IntentDigest           string        `json:"intent_digest"`
	Targets                []TargetEntry `json:"targets"`
	InputRepositoryWrites  int           `json:"input_repository_writes"`
	ApplyAuthorized        bool          `json:"apply_authorized"`
	CommitAuthorized       bool          `json:"commit_authorized"`
	PushAuthorized         bool          `json:"push_authorized"`
	PullRequestAuthorized  bool          `json:"pull_request_authorized"`
	MergeAuthorized        bool          `json:"merge_authorized"`
}

type ReplayReceipt struct {
	Schema                 string   `json:"schema"`
	Decision               string   `json:"decision"`
	Comparisons            int      `json:"comparisons"`
	Mismatches             int      `json:"mismatches"`
	ComparedArtifacts      []string `json:"compared_artifacts"`
	RollbackComparisons    int      `json:"rollback_comparisons"`
	RollbackMismatches     int      `json:"rollback_mismatches"`
	PatchRoundTrip         bool     `json:"patch_round_trip"`
	RollbackRoundTrip      bool     `json:"rollback_round_trip"`
	InputRepositoryWrites  int      `json:"input_repository_writes"`
}

type BundleManifest struct {
	Schema                 string        `json:"schema"`
	Version                string        `json:"version"`
	Decision               string        `json:"decision"`
	SourceTreeDigest       string        `json:"source_tree_digest"`
	IntentDigest           string        `json:"intent_digest"`
	ProposalDigest         string        `json:"proposal_digest"`
	AuthorityReceiptDigest string        `json:"authority_receipt_digest"`
	PreimageTreeDigest     string        `json:"preimage_tree_digest"`
	PostimageTreeDigest    string        `json:"postimage_tree_digest"`
	ChangedPaths           []string      `json:"changed_paths"`
	Findings               []Finding     `json:"findings"`
	Unknowns               []UnknownTuple `json:"unknowns"`
	Artifacts              []Artifact   `json:"artifacts"`
	Authority              Authority     `json:"authority"`
	Metrics                Metrics       `json:"metrics"`
}

type Artifact struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	Digest string `json:"digest"`
}

type Result struct {
	Manifest BundleManifest
	Files    map[string][]byte
}

type Options struct {
	SourceRoot     string
	SourceDigest   string
	ProposalPath   string
	AuthorityPath  string
	IntentPath     string
	ContractPath   string
	OutputDir      string
	ObservationPath string
}
