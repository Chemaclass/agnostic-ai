# Cross-model review experiment

**Status:** Protocol fixed, not run. [Issue #3837](https://github.com/Chemaclass/agnostic-ai/issues/3837) wrote it; epic [#3831](https://github.com/Chemaclass/agnostic-ai/issues/3831). Everything below is decided before any result is collected. A change to it after the first review is collected goes in the change log at the end, with its reason.

## Question

Does a review by a different model catch real bugs that a review by the same model misses?

## Sample

- 20 real changes from this repository. Each is named by an exact commit ID and an exact base commit ID, written in the sample table before any review runs.
- 5 changes with a bug planted on purpose. They live in a separate file, `cross-model-review-planted.md`, that the labeler does not open until labeling is done. They are scored on their own and never mixed into the 20.

| # | Commit | Base | Author model | Note |
|---|---|---|---|---|
| 1 | | | | |

## Arms

Every change gets two reviews:

1. **Same model:** a fresh session on the model that wrote the change.
2. **Other model:** a fresh session on a different model.

Fresh means a new session with no earlier context. Each arm gets one `review_request` mailbox message with the same refs (commit, base, paths) and nothing else: no brief, no question, no summary.

Write down per arm, before it runs:

| Change | Arm | Tool | Tool version | Model name | Reasoning effort | Built-ins installed |
|---|---|---|---|---|---|---|
| | | | | | | |

## Blind labeling

1. Collect every finding from both arms of a change.
2. Strip the source: no tool, model, arm, session or thread ID.
3. Shuffle the findings.
4. Label each one as **real bug**, **real but minor**, or **wrong**, without knowing its arm.
5. Only then join the labels back to their arms.

## What is compared

Per change, and summed over the 20:

- Real bugs found only by the other model.
- Real bugs found only by the same model.
- Wrong findings per arm.
- Messages and time per review, from `agnostic-ai mailbox stats` and the thread.

## Keep rule

Keep the feature if both hold:

- The other model finds real bugs the same-model review missed in **at least 3 of the 20** changes.
- The other model's wrong findings are **less than double** the same-model arm's.

Otherwise stop, and write down why in the results below.

## Waiting

- Record the median `deliverySeconds` from `agnostic-ai mailbox stats --since <first review date> --json`.
- Count the reviews where the thread was given up on, or the message was pasted by hand, because of the wait.
- Waiting counts as a real cost only when that happened in **at least 5 of the 25** reviews. Only then does [#3840](https://github.com/Chemaclass/agnostic-ai/issues/3840) (waking a session) start.

## Measured by hand

Time from turning the `mailbox` built-in on to the first delivered cross-model review, following the docs alone. Target: **under 10 minutes**. The store cannot know when the built-in was turned on; `reviews.firstThreadToFirstEmittedReviewSeconds` measures only from the first message in the store.

## Results

| Measure | Same model | Other model |
|---|---|---|
| Changes where only this arm found a real bug (of 20) | | |
| Real bugs found only by this arm | | |
| Real but minor points | | |
| Wrong findings | | |
| Median messages per review | | |
| Median minutes per review | | |
| Planted bugs found (of 5) | | |

| Waiting measure | Value |
|---|---|
| Median `deliverySeconds` | |
| Reviews given up on or pasted by hand (of 25) | |
| Minutes from built-in on to first delivered cross-model review | |

Decision:

## Change log

None yet.
