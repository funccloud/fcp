---
title: FCP Enhancement Proposal Process
authors:
  - "@felipeweb"
creation-date: 2025-03-24
status: proposed
---

# FCPEP-0001: FCP Enhancement Proposal Process

## Table of Contents

<!-- toc -->

- [Summary](#summary)
- [Motivation](#motivation)
- [Stewardship](#stewardship)
- [Reference-level explanation](#reference-level-explanation)
  - [What type of work should be tracked by a FCPEP](#what-type-of-work-should-be-tracked-by-a-fcpep)
  - [FCPEP Template](#fcpep-template)
  - [FCPEP Metadata](#fcpep-metadata)
  - [FCPEP Workflow](#fcpep-workflow)
  - [Git Implementation](#git-implementation)
  - [Prior Art](#prior-art)
- [Drawbacks](#drawbacks)
- [Unresolved Questions](#unresolved-questions)
<!-- /toc -->

## Summary

A standardized development process for FCP is proposed in order to

- provide a common structure and clear checkpoints for proposing
  changes to FCP
- ensure that the motivation for a change is clear
- allow for the enumeration stability milestones and stability graduation criteria
- persist project information in a Version Control System (VCS) for
  future FCP users and contributors
- support the creation of _high value user facing_ information such
  as:
  - an overall project development roadmap
  - motivation for impactful user facing changes
- ensure community participants are successfully able to drive changes
  to completion across one or more releases while stakeholders are
  adequately represented throughout the process

This process is supported by a unit of work called a FCP
Enhancement Proposal (FCPEP). A FCPEP attempts to combine aspects of the
following:

- feature, and effort tracking document
- a product requirements document
- design document

into one file which is created incrementally.

This process does not block authors from doing early design docs using
any means. It does not block authors from sharing those design docs
with the community (on Google Docs, Chat, GitHub, …).

**This process acts as a requirement when a design docs is ready to be
implemented or integrated in the `FCP` projects**. In other words,
a change that impacts other `FCP` projects or users cannot be
merged if there is no `FCPEP` associated with it. Bug fixes and small
changes like refactoring that do not affect the APIs (CRDs, REST APIs)
are not concerned by this. Fixing the behaviour of a malfunctioning
part of the project does not require a FCPEP.

This FCPEP process is related to

- the generation of an architectural roadmap
- the fact that the proposed feature is still undefined
- issue management
- the difference between an accepted design and a proposal
- the organization of design proposals

This proposal attempts to place these concerns within a general
framework.

## Motivation

For cross project or new project proposal, an abstraction beyond a
single GitHub issue seems to be required in order to understand and
communicate upcoming changes to the FCP community.

In a blog post describing the [road to Go 2][], Russ Cox explains

> that it is difficult but essential to describe the significance of a
> problem in a way that someone working in a different environment can
> understand

As a project, it is vital to be able to track the chain of custody for
a proposed enhancement from conception through implementation.

Without a standardized mechanism for describing important
enhancements, our talented technical writers and product managers
struggle to weave a coherent narrative explaining why a particular
release is important. Additionally, for critical infrastructure such
as FCP, adopters need a forward looking road map in order to plan
their adoption strategy.

The purpose of the FCPEP process is to ensure that the motivation
for a change is clear and the path is clear for all maintainers.

This is crucial to strengthen our product culture of building our products in public.
For our consultancy clients, we will continue to maintain full discretion and confidentiality about projects.

A FCPEP is broken into sections which can be merged into source control
incrementally in order to support an iterative development process. A
number of sections are required for a FCPEP to get merged in the
`proposed` state (see the different states in the [FCPEP
Metadata](#fcpep-metadata)). The other sections can be updated after
further discussions and agreement from the Working Groups.

[road to Go 2]: https://blog.golang.org/toward-go2

## Stewardship

The following
[DACI](https://en.wikipedia.org/wiki/Responsibility_assignment_matrix#DACI)
model indentifies the responsible parties for FCPEPs:

| **Workstream**            | **Driver**        | **Approver**          | **Contributor**                                      | **Informed** |
| ------------------------- | ----------------- | --------------------- | ---------------------------------------------------- | ------------ |
| FCPEP Process Stewardship | FCP Contributors  | FCP Governing members | FCP Contributors                                     | Community    |
| Enhancement delivery      | Enhancement Owner | Project(s) Owners     | Enhancement Implementer(s) (may overlap with Driver) | Community    |

In a nutshell, this means:

- Updates on the FCPEP process are driven by contributors and approved
  by the FCP governing board.
- Enhancement proposal are driven by contributors, and approved by the
  related project(s) owners.

## Reference-level explanation

### What type of work should be tracked by a FCPEP

The definition of what constitutes an "enhancement" is a foundational
concern for the FCP project. Roughly any FCP user or operator
facing enhancement should follow the FCPEP process. If an enhancement
would be described in either written or verbal communication to anyone
besides the FCPEP author or developer, then consider creating a
FCPEP. This means any change that may impact any other community project
in a way should be proposed as a FCPEP. Those changes could be for
technical reasons, or adding new features, or deprecating then
removing old features.

Similarly, any technical effort (refactoring, major architectural
change) that will impact a large section of the development community
should also be communicated widely. The FCPEP process is suited for this
even if it will have zero impact on the typical user or operator.

Project creations _or_ project promotion from the experimental project
would also fall under the FCPEP process.

### FCPEP Template

The template for a FCPEP is precisely defined
[here](.github/PULL_REQUEST_TEMPLATE/fcpep.md)

It's worth noting, the FCPEP template used to track API changes will
likely have different subsections than the template for proposing
governance changes. However, as changes start impacting other WGs or
the larger developer communities outside of a WG, the FCPEP process
should be used to coordinate and communicate.

### FCPEP Metadata

There is a place in each FCPEP for a YAML document that has standard
metadata. This will be used to support tooling around filtering and
display. It is also critical to clearly communicate the status of a
FCPEP.

Metadata items:

- **title** Required
  - The title of the FCPEP in plain language. The title will also be
    used in the FCPEP filename. See the template for instructions and
    details.
- **status** Required
  - The current state of the FCPEP.
  - Must be one of `proposed`, `implementable`,
    `implemented`,`withdrawn`, `deferred` or `replaced`.
- **authors** Required
  - A list of authors for the FCPEP. This is simply the github ID. In
    the future we may enhance this to include other types of
    identification.
- **creation-date** Required
  - The date that the FCPEP was first submitted in a PR.
  - In the form `yyyy-mm-dd`
  - While this info will also be in source control, it is helpful to
    have the set of FCPEP files stand on their own.
- **last-updated** Optional
  - The date that the FCPEP was last changed significantly.
  - In the form `yyyy-mm-dd`
- **see-also** Optional
  - A list of other FCPEPs that are relevant to this FCPEP.
  - In the form `FCPEP-123`
- **replaces** Optional
  - A list of FCPEPs that this FCPEP replaces. Those FCPEPs should list
    this FCPEP in their `superseded-by`.
  - In the form `FCPEP-123`
- **superseded-by** Optional
  - A list of FCPEPs that supersede this FCPEP. Use of this should be
    paired with this FCPEP moving into the `Replaced` status.
  - In the form `FCPEP-123`

### FCPEP Workflow

A FCPEP has the following states

- `proposed`: The FCPEP has been proposed and is actively being
  defined. This is the starting state while the FCPEP is being fleshed
  out and actively defined and discussed.
- `implementable`: The approvers have approved this FCPEP for
  implementation.
- `implemented`: The FCPEP has been implemented and is no longer
  actively changed. From that point on, the FCPEP should be considered
  _read-only_.
- `deferred`: The FCPEP is proposed but not actively being worked on
  and there are open questions.
- `withdrawn`: The FCPEP has been withdrawn by the authors or by the
  community on agreement with the authors.
- `replaced`: The FCPEP has been replaced by a new FCPEP. The
  `superseded-by` metadata value should point to the new FCPEP.

The workflow starts with a PR that introduces a new FCPEP in `proposed`
state. When the PR is merged, it means the project owners acknowledge
this is something we might want to work on _but_ the proposal needs
to be discussed and detailed before it can be accepted. The review
cycle on the initial PR should be short.

Once the FCPEP is `proposed`, the owners of the FCPEP (or someone else on
their behalf) shall submit a new PR that changes the status to
`implementable`, and present the FCPEP at a relevant working group, or
via the mailing list.

The discussion on the FCPEP shall be tracked on the PR, regardless of
the forum where it happens. We might need more information
about the impact on users, or some time to socialize it with the
Working Groups, etc.

The outcome may be that the FCPEP is approved, and moves to
`implementable`, or rejected, and moves to `withdrawn`. In case the
FCPEP is `withdrawn` it's best practice to update it with the reason
for withdrawal.

A FCPEP can be moved to the `implementable` state if it doesn't need
any more discussion and is approved as is.

A FCPEP can be marked `deferred` if it is not actively being worked on and
there are open questions. A `deferred` FCPEP can be moved to another state
with an explanation. If we answer the open questions and pick up the FCPEP,
it can be marked as `implementable`. If another FCPEP covers the use cases
and supersedes the FCPEP, then it can be marked as `replaced`. A `deferred`
FCPEP can be moved to its last state with an explanation and a discussion.

### Git Implementation

FCPEPs are checked into the community repo under the `/fcpeps` directory.

New FCPEPs can be checked in with a file name in the form of
`draft-YYYYMMDD-my-title.md`. As significant work is done on the FCPEP,
the authors can assign a FCPEP number. No other changes should be put in
that PR so that it can be approved quickly and minimize merge
conflicts. The FCPEP number can also be done as part of the initial
submission if the PR is likely to be uncontested and merged quickly.

### Prior Art

The FCPEP process as proposed was essentially adapted from the
[Kubernetes KEP process][], which itself is essentially stolen from the [Rust RFC
process][] which itself seems to be very similar to the [Python PEP
process][]

[Rust RFC process]: https://github.com/rust-lang/rfcs
[Kubernetes KEP process]: https://github.com/kubernetes/enhancements/tree/master/keps
[Python PEP process]: https://www.python.org/dev/peps/

## Drawbacks

Any additional process has the potential to engender resentment within
the community. There is also a risk that the FCPEP process as designed
will not sufficiently address the scaling challenges we face today. PR
review bandwidth is already at a premium and we may find that the FCPEP
process introduces an unreasonable bottleneck on our development
velocity.

The centrality of Git and GitHub within the FCPEP process also may place
too high a barrier to potential contributors, however, given that both
Git and GitHub are required to contribute code changes to FCP today
perhaps it would be reasonable to invest in providing support to those
unfamiliar with this tooling. It also makes the proposal document more
accessible than what it is before this proposal, as you are required
to be part of
[fcp-devs@](https://groups.google.com/a/funccloud.com/g/fcp-devs)
google groups to see the design docs.

## Unresolved Questions

- How reviewers and approvers are assigned to a FCPEP
- Example schedule, deadline, and time frame for each stage of a FCPEP
- Communication/notification mechanisms
- Review meetings and escalation procedure
