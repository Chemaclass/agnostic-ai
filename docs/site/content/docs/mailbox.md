+++
title = "Session mailbox"
description = "Send questions and findings between local Claude Code and Codex sessions from the companion."
weight = 80

[extra]
group = "Workflows"
+++

# Session mailbox

Keep a question or finding beside the two chats it concerns. The [companion](@/docs/cli-reference/companion-list.md) shows each thread under both local sessions.

## Send a message

1. Right-click a local Claude Code or Codex chat and choose **Send to session…**.
2. Choose **Question** or **Finding** and write your message.
3. Select a destination row or paste its [session ID](@/docs/cli-reference/companion-list.md#copy-a-session-id).
4. Send it, then open the thread to see the waiting message.

Choose **Type into session** to put it in the existing terminal tab. Press Enter in that tab when you are ready to send it. The Terminal app uses the clipboard to paste it. **Copy message** lets you paste it yourself in any app. A message marked **Staged** has been typed or copied by you. A failed attempt leaves it waiting.

## Manage a thread

| Action | What happens |
|---|---|
| **Pause** | Puts new sends and staging on hold until you resume it. |
| **Close thread** | Queues a final `done` message. The thread closes as soon as its waiting messages are staged. |
| **Stop** | Closes the thread right away and cancels waiting messages. Both sessions keep running. |
| **Allow 8 more** | Raises this thread’s limit after eight content messages. |

{% <details summary="Message format and refusals"> %}

A question or finding carries up to 4,000 characters. Each JSON message line fits in 5,000 bytes. The recipient sees JSON inside one `<agnostic-ai-mailbox>` block, with its thread, message number, type and sender. The wrapper labels the message as quoted data. The mailbox refuses text that looks like a credential.

A refusal gives the guard, reason and next step. For example, `cap` means the thread reached its limit. `unknown-session` means the destination is missing, ambiguous or outside the supported local sessions. See the [local API reference](@/docs/cli-reference/conversations-api.md#mailbox-routes) for the request format.

{% </details> %}

{% <details summary="Local storage and trust"> %}

The mailbox lives beside the conversation marks in the app’s configuration folder. It stores each thread separately and keeps a short record of refusals without message bodies. On macOS the folder is `~/Library/Application Support/agnostic-ai/mailbox/`.

The controls govern normal mailbox use. Any process running as your OS user can access files you own. A model may still act on text you paste, so read each message before submitting it.

{% </details> %}
