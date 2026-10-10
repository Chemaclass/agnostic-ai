# Final 24-case export privacy and fidelity review

PASS. The completed 24-case source has a fresh sanitized export with no observed privacy, provenance, or data-fidelity findings. This review does not assess answer quality, compare arms, or publish cost values. It opens no separate blind answer or verdict artifacts. The parent owns the blind review and subsequent analysis.

Source: `private-final-source`. New export: `sanitized-final-export`. Earlier source, pilot, exports and audit artifacts remain unchanged. No result row was omitted or changed in the measurement source.

The frozen current-main exporter generated 24 rows and 580 files. The public directory contains exactly the strict indexed audit whitelist plus its export manifest. All 579 original and sanitized indexed file hashes and byte counts match. All 24 framed hook requests match their original hashes and byte lengths. Cases and source row order are preserved, with no missing raw streams or incomplete-evidence flags.

All 4,820 parsed numeric and boolean values are preserved at their structural positions, including nested JSON strings and framed requests. All 360 selected measured/hash fields and complete usage, model-usage and token-accounting objects are unchanged. The audit does not print their values or make a correctness judgment. The 24 fixture bodies are byte-identical.

Across cases, 1,376 paired ID fields retain their relationships across 584 distinct per-case IDs. All 24 Bash request/result/hook links and all 12 selected Skill request/result links are verified. Public streams contain no raw opaque session/message/tool IDs matching the audited formats.

Private inventory checks verify 624 disabled private skill/plugin keys were masked and run 2,160 checks for collected private identities and fragments. Public native inventory remains intact: 84 skill entries and 72 builtin plugin entries, with Caveman present in the 12 selected cases. No private absolute path remains in inspected exported strings. Every public artifact passes the credential checks; only the exact public native settings singleton `AGNOSTIC_AI_TARGET=claude` is allowed as an environment field. No home/profile/auth/environment/store files are copied or opened by this export audit.

All six source assets match the pinned Caveman repository digests and source tree hash at revision `2e08b9177c07bb7249a8a2d1a6758e5db281d002`. The 72 selected public generated assets, including licenses and notices, remain exact. All 12 actual Skill load bodies match the trusted generated public body. All three recorded executable hashes match their actual binaries. The runner and exporter match the frozen current-main bytes.

The agnostic executable is an unreleased build from clean main `9932169b2d0836bea33144c0d9413052c488bddf`. Its reported version string is `agnostic-ai version 0.81.0`; that string does not identify the published 0.81.0 release. The published release lacks the new RTK/Caveman builtin names. Claude and RTK version strings are retained as provenance.

Verified provenance:

- baseline_commit: `9932169b2d0836bea33144c0d9413052c488bddf`.
- agnostic_build_commit: `9932169b2d0836bea33144c0d9413052c488bddf`.
- agnostic_version: `agnostic-ai version 0.81.0`.
- claude_version: `2.1.295 (Claude Code)`.
- rtk_version: `rtk 0.51.0`.
- runner_sha256: `828de40f154b2eb49d610125269fc206cfd30d41fdc1bd588684206a797b1d0a`.
- agnostic_binary_sha256: `81da49052803957ada1f8972e56c10f7c7e938d5a1db2201f7a6bfe29d09764b`.
- claude_binary_sha256: `0116ee2e0a513900b633d9951367f18747686478e2b462805b8c31609f047f70`.
- rtk_binary_sha256: `3aa2c36116310f1684f34ffd1dad023aa0930788d38219423ca5c1190c24fb7f`.
- skill_tree_sha256: `1cc150ef22276483226e03cd7718c43e80c232aa2d75b53bbd01d80042ac62eb`.
- private_original_manifest_sha256: `1b415dc142b468807202c5064d90fdaa9a0bcf522e0acbb6646684ed51ad907c`.
- private_original_results_sha256: `3dbe32ed35959f2805888d787fded0f687fdebf0a69f964af06206860de56ce6`.
- sanitized_manifest_sha256: `7337ec9db2a0a3dbf084d21866b1057402743376075b35bd77570298cb5531c7`.
- sanitized_results_sha256: `a85b7c2e7f99215aac5d12f1a47aa3eb73b8fdb8ccd3752f76f878129f8751e5`.
- export_manifest_sha256: `15808be4cf2ad28e1b2f07c2c0fb5d788da4be466fab1801b8e3d12d41e4f68a`.
- scratch_audit_sha256: `be1970abecbec828b5b30b64da5fa0283aa46eec1dca2bfc17a9008dc5b4d05d`.
- exporter_sha256: `30a9c1f7061f1da79c3f17a0ca3abc27692ab90718687daaa8ea7d34cef4c706`.

Inherited runner hashes identify private original bytes. The export index separately labels sanitized file hashes. Sanitizing does not remeasure provider usage, cost, or elapsed time.

Reproduce this read-only audit with:

```sh
python3 -B private-final-source-export-audit.py
```

Safe counts: `private-final-source-export-audit-counts.json`. Safe provenance: `private-final-source-export-provenance.json`. The new audit adapts the pilot audit without changing it.

No provider requests, builds, unit/full gates, commits, pushes, repository edits, or measurement changes were made. Privacy coverage is the selected whitelist, collected private identity/path/identifier formats, and credential checks; exact verified public pinned asset bodies are preserved.

Publication note: local source paths in this review are role labels. Scratch audit commands describe the original review environment. Use the verification and arithmetic commands in the main results report for the published artifacts.
