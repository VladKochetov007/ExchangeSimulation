package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"

	"exchange_sim/analysis"
	"exchange_sim/experiment/crossvenue"
)

type acceptedSummary struct {
	StudyID  string `json:"study_id"`
	Observed struct {
		CellResults []struct {
			Seed          int64  `json:"seed"`
			Arm           string `json:"arm"`
			ExecutionHash string `json:"execution_hash"`
			ResultSHA256  string `json:"result_sha256"`
		} `json:"cell_results"`
	} `json:"observed"`
}

type stageCell struct {
	Seed                  int64                           `json:"seed"`
	Arm                   string                          `json:"arm"`
	AcceptedResultSHA256  string                          `json:"accepted_result_sha256"`
	ExecutionHash         string                          `json:"execution_hash"`
	Public                analysis.CrossVenuePublicStages `json:"public"`
	RouterEvaluations     *uint64                         `json:"router_evaluations"`
	RouterSubmittedGroups *int                            `json:"router_submitted_groups"`
	FundingAndDelivery    string                          `json:"funding_and_delivery"`
}

type stageBatch struct {
	SchemaVersion  int         `json:"schema_version"`
	StudyID        string      `json:"study_id"`
	AnalysisCommit string      `json:"analysis_commit"`
	SummarySHA256  string      `json:"accepted_summary_sha256"`
	Cells          []stageCell `json:"cells"`
	Interpretation string      `json:"interpretation"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("me005stages", flag.ContinueOnError)
	root := flags.String("root", "", "retained ME-005 raw/rendered/result namespace")
	contracts := flags.String("contracts", "", "ME-005 contract directory")
	summaryPath := flags.String("summary", "", "accepted ME-005 machine result")
	output := flags.String("out", "", "new exploratory diagnostic file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *root == "" || *contracts == "" || *summaryPath == "" || *output == "" || flags.NArg() != 0 {
		return fmt.Errorf("me005stages: -root, -contracts, -summary and -out are required")
	}
	if strings.HasPrefix(filepath.Clean(*output), filepath.Clean(*root)+string(os.PathSeparator)) {
		return fmt.Errorf("me005stages: output cannot overwrite retained evidence namespace")
	}
	build, ok := debug.ReadBuildInfo()
	if !ok {
		return fmt.Errorf("me005stages: missing Go build identity")
	}
	settings := make(map[string]string, len(build.Settings))
	for _, setting := range build.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["vcs"] != "git" || settings["vcs.modified"] != "false" || len(settings["vcs.revision"]) != 40 {
		return fmt.Errorf("me005stages: requires clean pinned Go binary")
	}
	summaryRaw, err := os.ReadFile(*summaryPath)
	if err != nil {
		return err
	}
	var summary acceptedSummary
	if err := json.Unmarshal(summaryRaw, &summary); err != nil {
		return err
	}
	if summary.StudyID != "ME-005" || len(summary.Observed.CellResults) != 4 {
		return fmt.Errorf("me005stages: expected exactly four accepted ME-005 economic cells")
	}
	summaryHash := sha256.Sum256(summaryRaw)
	batch := stageBatch{SchemaVersion: 1, StudyID: "ME-005", AnalysisCommit: settings["vcs.revision"],
		SummarySHA256:  hex.EncodeToString(summaryHash[:]),
		Interpretation: "EXPLORATORY_PUBLIC_STAGE_DECOMPOSITION; REGISTERED_ME005_VERDICT_UNCHANGED"}
	seen := map[string]bool{}
	for _, accepted := range summary.Observed.CellResults {
		name := strings.ToLower(accepted.Arm) + "-" + fmt.Sprint(accepted.Seed)
		if !map[string]bool{"off-1109": true, "on-1109": true, "off-1117": true, "on-1117": true}[name] || seen[name] {
			return fmt.Errorf("me005stages: unexpected or duplicate cell %q", name)
		}
		seen[name] = true
		cell, err := diagnoseCell(*root, *contracts, name, accepted.Arm, accepted.Seed,
			accepted.ExecutionHash, accepted.ResultSHA256)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		batch.Cells = append(batch.Cells, cell)
	}
	encoded, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	written, writeErr := file.Write(encoded)
	if writeErr == nil && written != len(encoded) {
		writeErr = fmt.Errorf("short diagnostic write")
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func diagnoseCell(root, contracts, name, arm string, seed int64, executionHash, expectedResultHash string) (stageCell, error) {
	resultRaw, err := os.ReadFile(filepath.Join(root, name+"-result.json"))
	if err != nil {
		return stageCell{}, err
	}
	resultHash := sha256.Sum256(resultRaw)
	if hex.EncodeToString(resultHash[:]) != expectedResultHash {
		return stageCell{}, fmt.Errorf("retained result differs from accepted machine summary")
	}
	var result crossvenue.Result
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		return stageCell{}, err
	}
	contractRaw, err := os.ReadFile(filepath.Join(contracts, "contract-"+name+".json"))
	if err != nil {
		return stageCell{}, err
	}
	contract, err := crossvenue.DecodeContract(contractRaw)
	if err != nil {
		return stageCell{}, err
	}
	if contract.Seed != seed || contract.Arm != arm || result.Seed != seed || result.Arm != arm ||
		result.Binding.ExecutionStreamHash != executionHash || result.Binding.EffectiveConfigSHA256 != contract.EffectiveConfigSHA256 {
		return stageCell{}, fmt.Errorf("accepted result, contract and cell disagree")
	}
	maxAttempts := 0
	if arm == "ON" {
		maxAttempts = 1
	}
	rawDir, renderedDir := filepath.Join(root, name), filepath.Join(root, name+"-rendered")
	binding, err := analysis.VerifyCrossVenueRunBinding(rawDir, renderedDir, analysis.CrossVenueRunBindingExpectation{
		SourceRevision: contract.SourceRevision, EffectiveConfigSHA256: contract.EffectiveConfigSHA256,
		ExecutionStreamHash: executionHash, RouterEnabled: arm == "ON", Seed: seed,
		Venues: contract.Venues, LotQty: contract.LotQty, MaxAttempts: maxAttempts, TakerFeeBps: contract.TakerFeeBps,
	})
	if err != nil || !reflect.DeepEqual(binding, result.Binding) {
		return stageCell{}, fmt.Errorf("retained raw/rendered/result binding differs: %v", err)
	}
	var transitions []analysis.CrossVenuePublicTransition
	if arm == "ON" {
		if result.Router == nil || result.Router.Evidence == nil || result.Router.Evidence.PublicReplay == nil {
			return stageCell{}, fmt.Errorf("accepted ON result has no public replay")
		}
		transitions = result.Router.Evidence.PublicReplay.Transitions
	} else {
		if result.Router != nil {
			return stageCell{}, fmt.Errorf("accepted OFF result contains router")
		}
		run, err := analysis.Open(renderedDir)
		if err != nil {
			return stageCell{}, err
		}
		publicEvents, err := run.CollectCrossVenuePublicEvents(contract.Venues, contract.Symbol)
		if err != nil {
			return stageCell{}, err
		}
		replay, err := analysis.ReplayCrossVenuePublicEvents(publicEvents, analysis.CrossVenuePublicReplayOptions{
			Venues: contract.Venues, Symbol: contract.Symbol, InitiallyEmpty: true,
		})
		if err != nil {
			return stageCell{}, err
		}
		transitions = replay.Transitions
	}
	stages, err := analysis.DiagnoseCrossVenuePublicStages(contract.Venues, transitions,
		contract.HorizonNano, contract.LotQty, contract.BasePrecision, contract.TakerFeeBps)
	if err != nil {
		return stageCell{}, err
	}
	if stages.FeePositive.DirectionalEpisodes != 0 || stages.FeePositive.PositiveNanos != 0 ||
		result.Edge.EpisodeCount != 0 || result.Edge.PositiveNanos != 0 ||
		result.Edge.BroaderEpisodeCount != 0 {
		return stageCell{}, fmt.Errorf("exploratory stages contradict accepted zero-opportunity boundary")
	}
	cell := stageCell{Seed: seed, Arm: arm, AcceptedResultSHA256: expectedResultHash,
		ExecutionHash: executionHash, Public: stages,
		FundingAndDelivery: "NO_REGISTERED_FEE_POSITIVE_PUBLIC_EPISODE; CONDITIONAL_FUNDING_AND_DELIVERY_NOT_ESTIMATED"}
	if arm == "ON" {
		cell.RouterEvaluations = &result.Router.Evidence.Counters.QuoteEvaluations
		cell.RouterSubmittedGroups = &result.Router.Evidence.Counters.SubmittedGroups
	}
	return cell, nil
}
