package analysis

import "fmt"

// VerifyCrossVenueResponseActors binds each inbox receipt to the actor that
// actually evaluated the venue feed or submitted that leg. For a no-attempt
// venue without an evaluation, only same-client receipt consistency is known.
func VerifyCrossVenueResponseActors(evaluations []CrossVenueEvaluationRecord, groups []CrossVenueSubmissionGroup, receipts []CrossVenueResponseReceiptRecord, clients map[string]uint64) error {
	if len(clients) != 2 {
		return fmt.Errorf("cross-venue response actors: incomplete venue roster")
	}
	actors := make(map[string]uint64, 2)
	bind := func(venue string, clientID, actorID uint64) error {
		if venue == "" || clients[venue] != clientID || clientID == 0 || actorID == 0 {
			return fmt.Errorf("cross-venue response actors: unknown venue, client or actor")
		}
		if previous := actors[venue]; previous != 0 && previous != actorID {
			return fmt.Errorf("cross-venue response actors: %s actor identity changed", venue)
		}
		actors[venue] = actorID
		return nil
	}
	for _, evaluation := range evaluations {
		if err := bind(evaluation.Payload.TriggerVenueID, evaluation.Payload.TriggerClientID, evaluation.Payload.TriggerActorID); err != nil {
			return err
		}
	}
	for _, group := range groups {
		for _, leg := range []CrossVenueSubmissionLeg{group.Buy, group.Sell} {
			if err := bind(leg.Placement.Event.VenueID, leg.Decision.ClientID, leg.Decision.ActorID); err != nil {
				return err
			}
		}
	}
	for _, receipt := range receipts {
		if err := bind(receipt.Payload.VenueID, receipt.Payload.ClientID, receipt.Payload.ActorID); err != nil {
			return err
		}
	}
	return nil
}
