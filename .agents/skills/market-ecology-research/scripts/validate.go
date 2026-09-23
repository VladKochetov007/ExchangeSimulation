package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

type Authorization struct {
	Prepare bool   `json:"prepare"`
	Run     bool   `json:"run"`
	Basis   string `json:"basis"`
}
type Study struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	ClaimType     string        `json:"claim_type"`
	Stage         string        `json:"stage"`
	Idea          string        `json:"idea"`
	Protocol      *string       `json:"protocol"`
	Report        *string       `json:"report"`
	Result        *string       `json:"result"`
	Dependencies  []string      `json:"dependencies"`
	PolicyIDs     []string      `json:"policy_ids"`
	Authorization Authorization `json:"authorization"`
	NextAction    string        `json:"next_action"`
}
type Policy struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	SourceStatus    string   `json:"source_status"`
	Sources         []string `json:"sources"`
	CatalogueAnchor string   `json:"catalogue_anchor"`
}
type Registry struct {
	SchemaVersion  int      `json:"schema_version"`
	BaselineCommit string   `json:"baseline_commit"`
	Studies        []Study  `json:"studies"`
	Policies       []Policy `json:"policies"`
}
type Review struct {
	Execution string  `json:"execution"`
	Verdict   string  `json:"verdict"`
	Scope     *string `json:"scope"`
	Reference *string `json:"reference"`
}
type Claim struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Text        string   `json:"text"`
	EvidenceIDs []string `json:"evidence_ids"`
	Limitations []string `json:"limitations"`
}
type Evidence struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Location string `json:"location"`
	Identity string `json:"identity"`
}
type Result struct {
	SchemaVersion        int                `json:"schema_version"`
	StudyID              string             `json:"study_id"`
	Stage                string             `json:"stage"`
	ProcessStatus        string             `json:"process_status"`
	EvidenceValidity     string             `json:"evidence_validity"`
	OpportunityPresence  string             `json:"opportunity_presence"`
	PolicyActivity       string             `json:"policy_activity"`
	RegisteredActivation string             `json:"registered_activation"`
	ScientificVerdict    string             `json:"scientific_verdict"`
	CausalVerdict        string             `json:"causal_verdict"`
	EmpiricalComparison  string             `json:"empirical_comparison"`
	Reasons              []string           `json:"reasons"`
	Identities           map[string]*string `json:"identities"`
	Authorization        Authorization      `json:"authorization"`
	Review               Review             `json:"review"`
	Claims               []Claim            `json:"claims"`
	Evidence             []Evidence         `json:"evidence"`
}

const skillPath = ".agents/skills/market-ecology-research"
const claimTypes = "MECHANICAL DESCRIPTIVE ACTIVATION CAUSAL REPLICATION EMPIRICAL GAME-THEORETIC"
const stages = "INTAKE PLAN PREPARE DEVELOPMENT CONFIRMATION REPORT CLOSED"

var studyID = regexp.MustCompile(`^ME-[0-9]{3}(-[A-Z0-9]+)*$`)
var policyID = regexp.MustCompile(`^S[0-9]{2,}$`)
var commitID = regexp.MustCompile(`^[0-9a-f]{40}$`)
var markdownLink = regexp.MustCompile(`\]\(([^)]+)\)`)

func member(value, choices string) bool {
	for _, choice := range strings.Fields(choices) {
		if value == choice {
			return true
		}
	}
	return false
}

func decode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	return requireFields(data, reflect.TypeOf(target).Elem())
}

// Required-but-null fields distinguish a pending identity from omitted metadata.
func requireFields(data []byte, shape reflect.Type) error {
	if shape.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			return nil
		}
		return requireFields(data, shape.Elem())
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("unexpected null for %s", shape)
	}
	switch shape.Kind() {
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return err
		}
		for index := 0; index < shape.NumField(); index++ {
			field := shape.Field(index)
			name := field.Tag.Get("json")
			raw, exists := fields[name]
			if !exists {
				return fmt.Errorf("missing field %s", name)
			}
			if err := requireFields(raw, field.Type); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	case reflect.Slice:
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		for _, item := range items {
			if err := requireFields(item, shape.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := decode(data, target); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func repositoryFile(root, reference string) error {
	path, anchor, _ := strings.Cut(reference, "#")
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "\\") {
		return fmt.Errorf("invalid repository reference %q", reference)
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("reference escapes repository: %s", reference)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, clean))
	if err != nil {
		return fmt.Errorf("missing reference %s: %w", reference, err)
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("reference resolves outside repository: %s", reference)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("reference is not a regular file: %s", reference)
	}
	if anchor != "" {
		data, err := os.ReadFile(resolved)
		if err != nil {
			return err
		}
		if !bytes.Contains(data, []byte(`id="`+anchor+`"`)) {
			return fmt.Errorf("missing explicit anchor: %s", reference)
		}
	}
	return nil
}

func validateRegistry(root string, registry Registry) error {
	if registry.SchemaVersion != 1 || !commitID.MatchString(registry.BaselineCommit) {
		return fmt.Errorf("invalid registry version or baseline")
	}
	policies := map[string]bool{}
	for _, policy := range registry.Policies {
		if !policyID.MatchString(policy.ID) || policies[policy.ID] || policy.Title == "" {
			return fmt.Errorf("invalid/duplicate policy %s", policy.ID)
		}
		policies[policy.ID] = true
		if !member(policy.SourceStatus, "IMPLEMENTED PARTIAL NOT_IMPLEMENTED UNVERIFIED") {
			return fmt.Errorf("invalid policy source status %s", policy.ID)
		}
		if member(policy.SourceStatus, "IMPLEMENTED PARTIAL") && len(policy.Sources) == 0 {
			return fmt.Errorf("source claim without reference: %s", policy.ID)
		}
		for _, path := range append(append([]string{}, policy.Sources...), policy.CatalogueAnchor) {
			if err := repositoryFile(root, path); err != nil {
				return err
			}
		}
	}
	studies := map[string]Study{}
	for _, study := range registry.Studies {
		if !studyID.MatchString(study.ID) || study.Title == "" || study.NextAction == "" {
			return fmt.Errorf("invalid study %s", study.ID)
		}
		if _, exists := studies[study.ID]; exists {
			return fmt.Errorf("duplicate study %s", study.ID)
		}
		studies[study.ID] = study
		if !member(study.ClaimType, claimTypes) || !member(study.Stage, stages) || study.Authorization.Basis == "" {
			return fmt.Errorf("invalid study contract %s", study.ID)
		}
		if study.Authorization.Run && study.Protocol == nil {
			return fmt.Errorf("run authorized without protocol: %s", study.ID)
		}
		paths := []string{study.Idea}
		for _, path := range []*string{study.Protocol, study.Report, study.Result} {
			if path != nil {
				paths = append(paths, *path)
			}
		}
		for _, path := range paths {
			if err := repositoryFile(root, path); err != nil {
				return err
			}
		}
		for _, id := range study.PolicyIDs {
			if !policies[id] {
				return fmt.Errorf("missing policy %s for %s", id, study.ID)
			}
		}
		if study.Result != nil {
			var result Result
			if err := readJSON(filepath.Join(root, *study.Result), &result); err != nil {
				return err
			}
			if result.StudyID != study.ID || result.Stage != study.Stage {
				return fmt.Errorf("registry/result identity or stage mismatch: %s", study.ID)
			}
			if err := validateResult(result); err != nil {
				return err
			}
		}
	}
	visited, active := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		study, exists := studies[id]
		if !exists || active[id] {
			return fmt.Errorf("missing or cyclic dependency %s", id)
		}
		if visited[id] {
			return nil
		}
		active[id] = true
		for _, dependency := range study.Dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		active[id], visited[id] = false, true
		return nil
	}
	for id := range studies {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func validateResult(result Result) error {
	if result.SchemaVersion != 1 || !studyID.MatchString(result.StudyID) {
		return fmt.Errorf("invalid result version/ID")
	}
	fields := []struct{ value, choices string }{
		{result.Stage, stages},
		{result.ProcessStatus, "NOT_RUN COMPLETED FAILED INCOMPLETE"},
		{result.EvidenceValidity, "NOT_ASSESSED VALID INVALID INCOMPLETE"},
		{result.OpportunityPresence, "NOT_ASSESSED PRESENT ABSENT UNKNOWN NOT_APPLICABLE"},
		{result.PolicyActivity, "NOT_ASSESSED ACTIVE INACTIVE UNKNOWN NOT_APPLICABLE"},
		{result.RegisteredActivation, "NOT_ASSESSED SATISFIED NOT_SATISFIED UNKNOWN NOT_APPLICABLE"},
		{result.ScientificVerdict, "NOT_ISSUED SUPPORTED NOT_SUPPORTED INCONCLUSIVE IDENTIFICATION_LIMITATION MECHANICAL_ONLY"},
		{result.CausalVerdict, "NOT_ASSESSED SUPPORTED NOT_SUPPORTED INCONCLUSIVE NOT_IDENTIFIED NOT_APPLICABLE"},
		{result.EmpiricalComparison, "NOT_PERFORMED COMPATIBLE MISMATCH INCONCLUSIVE NOT_APPLICABLE"},
		{result.Review.Execution, "COMPLETED UNAVAILABLE FAILED NOT_REQUESTED"},
		{result.Review.Verdict, "ACCEPT ACCEPT_WITH_REQUIRED_CHANGES REJECT NOT_ISSUED"},
	}
	for _, field := range fields {
		if !member(field.value, field.choices) {
			return fmt.Errorf("invalid result enum %q", field.value)
		}
	}
	if len(result.Reasons) == 0 || result.Authorization.Basis == "" {
		return fmt.Errorf("result requires reasons and authorization basis")
	}
	identityKeys := strings.Fields("source configuration run analyzer protocol skill_commit skill_tree")
	if len(result.Identities) != len(identityKeys) {
		return fmt.Errorf("incorrect identity fields")
	}
	for _, key := range identityKeys {
		value, exists := result.Identities[key]
		if !exists || (value != nil && *value == "") {
			return fmt.Errorf("invalid identity %s", key)
		}
	}
	if result.ScientificVerdict != "NOT_ISSUED" && (result.EvidenceValidity != "VALID" || result.ProcessStatus != "COMPLETED") {
		return fmt.Errorf("verdict without complete valid evidence")
	}
	if len(result.Claims) > 0 && (result.EvidenceValidity != "VALID" || result.ProcessStatus != "COMPLETED") {
		return fmt.Errorf("claims without complete valid evidence")
	}
	if member(result.CausalVerdict, "SUPPORTED NOT_SUPPORTED INCONCLUSIVE NOT_IDENTIFIED") && (result.ScientificVerdict == "NOT_ISSUED" || result.EvidenceValidity != "VALID") {
		return fmt.Errorf("causal verdict without valid scientific result")
	}
	if result.ScientificVerdict == "MECHANICAL_ONLY" && !member(result.CausalVerdict, "NOT_ASSESSED NOT_APPLICABLE") {
		return fmt.Errorf("mechanical-only result cannot issue a causal verdict")
	}
	if result.ScientificVerdict != "NOT_ISSUED" {
		if len(result.Claims) == 0 {
			return fmt.Errorf("verdict without evidence-linked claims")
		}
		for _, key := range identityKeys {
			if result.Identities[key] == nil {
				return fmt.Errorf("issued verdict lacks identity %s", key)
			}
		}
	}
	if member(result.EmpiricalComparison, "COMPATIBLE MISMATCH INCONCLUSIVE") && (result.EvidenceValidity != "VALID" || len(result.Evidence) == 0) {
		return fmt.Errorf("empirical comparison without valid evidence")
	}
	if result.ProcessStatus == "NOT_RUN" && (len(result.Claims) != 0 || result.EvidenceValidity != "NOT_ASSESSED") {
		return fmt.Errorf("no-run record contains result claims")
	}
	if result.Review.Execution != "COMPLETED" && result.Review.Verdict != "NOT_ISSUED" {
		return fmt.Errorf("review verdict without completed review")
	}
	if result.Review.Verdict != "NOT_ISSUED" && (result.Review.Scope == nil || result.Review.Reference == nil || *result.Review.Scope == "" || *result.Review.Reference == "") {
		return fmt.Errorf("substantive review lacks scope/reference")
	}
	evidence := map[string]bool{}
	for _, entry := range result.Evidence {
		if entry.ID == "" || evidence[entry.ID] || entry.Kind == "" || entry.Location == "" || entry.Identity == "" {
			return fmt.Errorf("invalid/duplicate evidence %q", entry.ID)
		}
		evidence[entry.ID] = true
	}
	claims := map[string]bool{}
	for _, claim := range result.Claims {
		if claim.ID == "" || claims[claim.ID] || !member(claim.Type, claimTypes) || claim.Text == "" || len(claim.EvidenceIDs) == 0 || len(claim.Limitations) == 0 {
			return fmt.Errorf("invalid claim %q", claim.ID)
		}
		claims[claim.ID] = true
		for _, id := range claim.EvidenceIDs {
			if !evidence[id] {
				return fmt.Errorf("claim %s references missing evidence %s", claim.ID, id)
			}
		}
	}
	return nil
}

func validateSkill(root string) error {
	data, err := os.ReadFile(filepath.Join(root, skillPath, "SKILL.md"))
	if err != nil {
		return err
	}
	parts := strings.SplitN(string(data), "---\n", 3)
	if len(parts) != 3 || parts[0] != "" {
		return fmt.Errorf("missing skill frontmatter")
	}
	lines := strings.Split(strings.TrimSpace(parts[1]), "\n")
	if len(lines) != 2 || lines[0] != "name: market-ecology-research" || !strings.HasPrefix(lines[1], "description: ") || len(lines[1]) < 40 {
		return fmt.Errorf("unsupported skill frontmatter; expected name and single-line description")
	}
	metadata, err := os.ReadFile(filepath.Join(root, skillPath, "agents/openai.yaml"))
	if err != nil {
		return err
	}
	expected := regexp.MustCompile(`(?s)^interface:\n  display_name: "[^"\n]+"\n  short_description: "[^"\n]{25,64}"\n  default_prompt: "[^"\n]*\$market-ecology-research[^"\n]*"\npolicy:\n  allow_implicit_invocation: false\n$`)
	if !expected.Match(metadata) {
		return fmt.Errorf("invalid explicit-only metadata for this skill's supported YAML shape")
	}
	return nil
}

func validateLinks(root, directory string) error {
	return filepath.WalkDir(filepath.Join(root, directory), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".md" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range markdownLink.FindAllStringSubmatch(string(data), -1) {
			target := match[1]
			if strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "http://") {
				continue
			}
			relative, err := filepath.Rel(root, filepath.Join(filepath.Dir(path), target))
			if err != nil {
				return err
			}
			if err := repositoryFile(root, relative); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
		}
		return nil
	})
}

func validate(root string) error {
	var registry Registry
	if err := readJSON(filepath.Join(root, "research/program/registry.json"), &registry); err != nil {
		return err
	}
	if err := validateRegistry(root, registry); err != nil {
		return err
	}
	if err := validateSkill(root); err != nil {
		return err
	}
	var template Result
	if err := readJSON(filepath.Join(root, skillPath, "assets/result-template.json"), &template); err != nil {
		return err
	}
	if err := validateResult(template); err != nil {
		return err
	}
	for _, directory := range []string{skillPath, "research/program"} {
		if err := validateLinks(root, directory); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	rootFlag := flag.String("root", ".", "repository root; only metadata and linked source existence are checked")
	flag.Parse()
	root, err := filepath.Abs(*rootFlag)
	if err == nil {
		err = validate(root)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Skill metadata, registry, result template and local links: PASS")
}
