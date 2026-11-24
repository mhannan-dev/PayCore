# Hackathon eligibility decision

**Status: NOT CONFIRMED**

The QDay builder challenge may be entered only with core building performed during
the build window, 2026-11-16 through 2026-12-11. The official FAQ states:

> "Adding tUSD8 to an existing product, or submitting an existing product as it is,
> doesn't count as a submission."

Source: <https://qday.io/builder-challenge> (official challenge page, checked
2026-11-23).

## Decision

The current branch is an extension of the pre-existing PayCore repository.
Therefore, submitting the branch as "PayCore Quantum" without organizer approval
is **not confirmed eligible**. No written approval has been obtained from the
organizers.

Before submission, the team must select one of these options:

1. **Written approval (preferred for this repository):** Ask QDay in Discord or
   through the DoraHacks submission page whether a new Quantum implementation
   built during the build window may reuse MIT-licensed PayCore as permissible
   boilerplate. Use [ORGANIZER_REQUEST.md](ORGANIZER_REQUEST.md). Save the
   written response and name the organizer who gave it.
2. **Standalone clean-room project:** Create a new repository. Write the core
   contracts, settlement flow, agent mandate and demo code during the build
   window. Reuse only clearly generic PayCore pieces, preserve their MIT notices
   and license, add source comments above copied snippets, and add a `NOTICE`
   file identifying all reused components.

## Provenance

The existing PayCore code predates this challenge. The challenge-specific work
starts with the Quantum branch implementation and the `QuantumPayments`
contract. The repository must describe both:

- baseline PayCore features reused unchanged or with only integration changes;
- Quantum features and contract code created for the challenge build window;
- any third-party boilerplate or libraries, with their original license.

The `QuantumPayments` contract is MIT licensed through the repository license.
Copied or adapted starter-kit code must retain its original SPDX identifier,
copyright notice and license.

## Submission gate

Do not mark this project eligible until at least one of these is true:

- official written approval for extending PayCore is archived in the submission;
- or a separate standalone repository satisfying the clean-room option exists
  and is the submitted repository.
