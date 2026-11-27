+++
title = "Pin a Companion chat"
description = "Keep a chat at the top of the Companion list while it works or after it ends."
weight = 47

[extra]
group = "Reference"
+++

# Pin a Companion chat

Right-click a top-level chat in the [Companion list](@/docs/cli-reference/companion-list.md) and choose **Pin to top**. With the row selected, Shift+F10 opens the same menu. The chat moves to **Pinned** without opening or marking it read. Its visible child chats come with it.

Pinned chats appear as soon as the list opens, even if their last message was weeks ago. The order follows when you pinned them. Search and the **All**, **Local** or **Cloud** choice still filter the list.

A chat stays pinned after it ends, after you mark it complete and after a restart. Choose **Unpin** from its menu or click the pin beside its title to put it back in its state group. Hiding or deleting the chat also clears the pin.

The [conversation marks file](@/docs/cli-reference/conversations-api.md#get-api-views-conversations) saves the pin as `pinnedAt`.
