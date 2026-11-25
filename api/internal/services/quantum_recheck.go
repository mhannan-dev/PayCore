package services

import (
 "time"
 "paycore/api/internal/models"
)

// Preserve the accepted proof on transient negatives; only a conflict or
// sustained absence invalidates claims and emits reorg events.
func recheckQuantumConfirmation(w models.QuantumWork, r models.QuantumReceipt, minimum int64, now time.Time, skew time.Duration) models.QuantumDecision {
 reorg:=models.QuantumDecision{Status:models.QuantumReorged,Reorg:true,Receipt:r,RetryAfter:3*time.Second}
 if r.Found && (!r.Canonical || (w.ObservedHash!=nil && *w.ObservedHash!=r.BlockHash)) { return reorg }
 keep:=models.QuantumDecision{Status:models.QuantumConfirmed,Receipt:r,RetryAfter:15*time.Second,PreserveObservation:true}
 if !r.Found {
  since:=w.ReorgSince
  if since==nil { start:=now;since=&start }
  count:=min(w.ReorgSuspicions+1,5)
  if count>=5 && now.Sub(*since)>=time.Minute { return reorg }
  keep.ReorgSuspicions=count;keep.ReorgSince=since
  return keep
 }
 // A lower head is a lagging RPC, not evidence that a canonical block changed.
 if r.Confirmations<w.Attempt.Confirmations || r.Confirmations<minimum { return keep }
 decision:=evaluateQuantumReceipt(w,r,minimum,now,skew)
 if decision.Status!=models.QuantumConfirmed {
  missing:=models.QuantumReceipt{ChainID:r.ChainID}
  return recheckQuantumConfirmation(w,missing,minimum,now,skew)
 }
 decision.Finalized=r.Confirmations>=max(minimum,64) && w.ConfirmedAt!=nil && now.Sub(*w.ConfirmedAt)>10*time.Minute
 return decision
}
