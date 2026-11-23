# Cross-model review experiment

**Status:** Ran 50 reviews on 2026-11-23. [Issue #3837](https://github.com/Chemaclass/agnostic-ai/issues/3837) wrote it; epic [#3831](https://github.com/Chemaclass/agnostic-ai/issues/3831). The sample and keep rule were set before collecting reviews. The change log records run adjustments.

## Question

Does a review by a different model catch real bugs that a review by the same model misses?

## Sample

- 20 real changes from this repository. Each is named by an exact commit ID and an exact base commit ID, written in the sample table before any review runs.
- 5 changes with a bug planted on purpose. The [planted changes](cross-model-review-planted.md) were sealed until labeling finished. They are scored on their own and never mixed into the 20.

| # | Commit | Base | Author model | PR | Arm with a unique material finding |
|---|---|---|---|---|---|
| 1 | `125f11b2675d1bf1c124364b11c4ecb283386563` | `8c8fd5ecfab64bc97b465e3bdfed6a49894bbb51` | `claude-opus-5-5` | [#2989](https://github.com/Chemaclass/agnostic-ai/pull/2989) | None |
| 2 | `4fd38607c909f79d6aa594e9528f682787d68469` | `3777784f3786e789d5bf874083ff8eda5fb645ba` | `gpt-6.2-sol` | [#3796](https://github.com/Chemaclass/agnostic-ai/pull/3796) | None |
| 3 | `10b050ed58e6d4c2847ea17af4410ff9babde523` | `125f11b2675d1bf1c124364b11c4ecb283386563` | `claude-opus-5-5` | [#2992](https://github.com/Chemaclass/agnostic-ai/pull/2992) | Other model |
| 4 | `af49d6ce10aaf4233d24806d1c39739a2094bece` | `c06f659bde0704b3eceb8d74d6bae70bde36dc2f` | `gpt-6.2-sol` | [#3802](https://github.com/Chemaclass/agnostic-ai/pull/3802) | None |
| 5 | `8edd134eb68e00b0bb7d3b2cc936b308445c5d5e` | `10b050ed58e6d4c2847ea17af4410ff9babde523` | `claude-opus-5-5` | [#3001](https://github.com/Chemaclass/agnostic-ai/pull/3001) | None |
| 6 | `1ea32c34eb202b41508fe2eaa40ea3e07088ea23` | `af49d6ce10aaf4233d24806d1c39739a2094bece` | `gpt-6.2-sol` | [#3804](https://github.com/Chemaclass/agnostic-ai/pull/3804) | None |
| 7 | `8a9e8eecc030238c24ce7b682b362feec2941495` | `8e27516a4591da892093b3af0bfa6d0244a8e278` | `claude-opus-5-5` | [#3194](https://github.com/Chemaclass/agnostic-ai/pull/3194) | None |
| 8 | `3f854a980372d0db69b8e9c25a8684a74de1b70d` | `be50919e5bd40220883eeb157bd57f5d5ed1bc9b` | `gpt-6.2-sol` | [#3821](https://github.com/Chemaclass/agnostic-ai/pull/3821) | None |
| 9 | `c4d0c590a47b112fc786772134eed3b3fe415e73` | `adb550168c8382e9fbd2c903dc6e726d9e7e733c` | `claude-opus-5-5` | [#3199](https://github.com/Chemaclass/agnostic-ai/pull/3199) | None |
| 10 | `b116f455d190820fa8c1ec68226d74029bcda439` | `612d267e9baa19291a788552ef69a7c278a78217` | `gpt-6.2-sol` | [#3829](https://github.com/Chemaclass/agnostic-ai/pull/3829) | None |
| 11 | `1118873744f3e3a239a85527ca546a05e48de41d` | `b2b5f9a1aa43236c33061933dbca6977e6a448c4` | `claude-opus-5-5` | [#3241](https://github.com/Chemaclass/agnostic-ai/pull/3241) | Other model |
| 12 | `8777d890f0a038e6a327dad62489fea4642cae8d` | `97b670ce7afe60d072843d2637a76d424d9c699b` | `gpt-6.2-sol` | [#3841](https://github.com/Chemaclass/agnostic-ai/pull/3841) | Other model |
| 13 | `ed0eccd35bc3c7361f762e43116d8b175a288af0` | `4770f651d8045f39e952377b4583b1c8f382fd18` | `claude-opus-5-5` | [#3258](https://github.com/Chemaclass/agnostic-ai/pull/3258) | Other model |
| 14 | `074973f495583e49c1e17814880c9bc81f2df0c0` | `8777d890f0a038e6a327dad62489fea4642cae8d` | `gpt-6.2-sol` | [#3842](https://github.com/Chemaclass/agnostic-ai/pull/3842) | Both |
| 15 | `dc137976fecebbeca19b5a2c5e1073ec4e4742a3` | `f899c09e135396ec18650a85f85cddaeb7359473` | `claude-opus-5-5` | [#3607](https://github.com/Chemaclass/agnostic-ai/pull/3607) | None |
| 16 | `edbfc1dc3d0eb63d3ad9eda101560fab6fb1b9a8` | `074973f495583e49c1e17814880c9bc81f2df0c0` | `gpt-6.2-sol` | [#3851](https://github.com/Chemaclass/agnostic-ai/pull/3851) | Same model |
| 17 | `59cc492dd57b8afae25d8977acd34ba4774bb9f0` | `580351abe09d0516cb87cba4a0fc787efe874864` | `claude-opus-5-5` | [#3807](https://github.com/Chemaclass/agnostic-ai/pull/3807) | Other model |
| 18 | `710c0e85c540a3ca21b9c07739030489116d8a77` | `edbfc1dc3d0eb63d3ad9eda101560fab6fb1b9a8` | `gpt-6.2-sol` | [#3853](https://github.com/Chemaclass/agnostic-ai/pull/3853) | None |
| 19 | `0738ab39ee725194c266b726e7b2e95a065d4415` | `45737a6bc95e4aa052ce535fa9984894f8c1437f` | `claude-opus-5-5` | [#3953](https://github.com/Chemaclass/agnostic-ai/pull/3953) | Other model |
| 20 | `6cfc29ab91ebb46c3c0b3b626171cc7c8ca53695` | `d987946077a4bea6daad85648dcf9c5ac8f8c035` | `gpt-6.2-sol` | [#3958](https://github.com/Chemaclass/agnostic-ai/pull/3958) | Both |

## Arms

Every change gets two reviews:

1. **Same model:** a fresh session on the model that wrote the change.
2. **Other model:** a fresh session on a different model.

Fresh means a new session with no earlier context. Each arm gets one `review_request` mailbox message with the same refs (commit, base, paths) and nothing else: no brief, no question, no summary.

Write down per arm, before it runs:

| Tool | Tool version | Model name | Reasoning effort | Built-ins installed |
|---|---|---|---|---|
| Codex CLI | 0.171.0 | `gpt-6.2-sol` | medium | `mailbox` |
| Claude Code | 2.1.330 | `claude-opus-5-5` | medium | `mailbox` |

These settings apply to every run. The sample names the model for the same-model arm. The other model handled the paired review. The five planted changes had preassigned arm names.

The run used agnostic-ai v0.98.0 on macOS arm64. Local session records tied the author model to each pull request or its head commit. Each reviewer started in a separate session from the pinned change, with the same review instruction. The three refs fields arrived through the mailbox hook. A local driver queued the requests in a scratch mailbox and captured final replies outside the mailbox. Each scored thread therefore held one message.

Codex used a read-only workspace. Claude Code could read files and run selected read-only Git commands. Tool restrictions differed, so this compares the two review setups as used here.

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
| Changes with a material finding unique to this arm (of 20) | 3 | 8 |
| Real bugs found only by this arm | 8 | 15 |
| Real but minor points | 12 | 8 |
| Wrong or unsupported findings | 0 | 3 |
| Median messages per review | 1 | 1 |
| Median minutes from queue to final reply | 1.02 | 0.94 |
| Planted bugs found (of 5) | 5 | 5 |

| Waiting measure | Value |
|---|---|
| Median `deliverySeconds` | 1.68 seconds across 50 delivered requests |
| Changes with a review abandoned or pasted by hand (of 25) | 0 |
| Minutes from built-in on to first delivered cross-model review | 4:58 in the Companion walkthrough |

The [raw mailbox counts](cross-model-review-stats.json) include 52 requests. Two Codex requests did not deliver because the scratch setup trusted the worktree path and Codex used the common Git checkout path. Fresh sessions received the same requests after that was corrected. Both undelivered attempts are excluded from scoring.

The driver started reviewers when their requests were ready. It queued Claude Code requests before launching the process and Codex requests after creating the thread. The reply timings include that startup difference. Waiting for a person to return to an idle session remains unmeasured.

### Setup walkthrough

On macOS arm64, agnostic-ai v0.98.0 delivered a review request in 4 minutes 58 seconds from turning on the built-in in a fresh Git project. Both CLIs were already signed in. The setup followed the [mailbox guide](https://agnostic-ai.org/docs/mailbox/) and trusted both generated hooks in Codex's `/hooks` menu. Scripted clicks in an isolated native Companion window opened **Message another session…**, selected **Review request**, previewed the base and commit, and sent the request from Claude Code to Codex. The Codex prompt hook delivered it on the next prompt. Codex used the previewed base, commit and two paths in its diff command.

The time includes trying Claude Code in print mode and reopening it interactively when the chat did not appear in the Companion. The scratch mailbox recorded one delivered cross-model request and 37.24 seconds from queuing to delivery. This setup measurement is separate from the 50 scored reviews. The earlier driver-based pilot took 4 minutes.

## Decision

The other model met the minimum for finding bugs the same model missed. It failed the limit on wrong findings.

Stop expanding the cross-model review workflow on this evidence. Leave the [session wake investigation](https://github.com/Chemaclass/agnostic-ai/issues/3840) deferred.

## Change log

- 2026-11-23. Corrected the Codex hook trust location after two setup attempts. The sample, scoring and keep rule stayed fixed.
- 2026-11-23. Timed the Companion walkthrough after the driver-based pilot.
