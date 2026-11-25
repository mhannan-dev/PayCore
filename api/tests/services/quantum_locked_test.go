package services_test

import (
 "testing"
 "time"

 "paycore/api/internal/models"
 "paycore/api/internal/services"
)

func TestQuantumF2TransientEvidenceDoesNotReorg(t *testing.T) {
 for _, name := range []string{"missing", "lower head"} {
  t.Run(name, func(t *testing.T) {
   w, r, now := quantumProof()
   w.Attempt.Status = models.QuantumConfirmed
   w.Attempt.Confirmations = 12
   w.ObservedHash = &r.BlockHash
   if name == "missing" { r.Found = false } else { r.Confirmations = 1 }
   d := services.EvaluateQuantumReceipt(w, r, 2, now)
   if d.Reorg || d.Status != models.QuantumConfirmed { t.Fatalf("transient evidence changed paid status: %+v", d) }
  })
 }
}

func TestQuantumF4ClockSkew(t *testing.T) {
 for _, contract := range []bool{false,true} {
  for _, tc := range []struct{name string; offset time.Duration; want models.QuantumAttemptStatus}{
   {"30 seconds before", -30*time.Second,models.QuantumConfirmed},
   {"two minutes before",-2*time.Minute,models.QuantumReview},
   {"exact lower bound",-time.Minute,models.QuantumConfirmed},
  } {
   t.Run(tc.name+map[bool]string{false:" direct",true:" contract"}[contract],func(t *testing.T){
    w,r,now:=quantumProof()
    if contract { w,r=contractProof() }
    w.Attempt.CreatedAt=now.Truncate(time.Second)
    r.BlockTime=w.Attempt.CreatedAt.Add(tc.offset)
    d:=services.EvaluateQuantumReceipt(w,r,2,now)
    if d.Status!=tc.want { t.Fatalf("skew decision: %+v",d) }
   })
  }
 }
}
