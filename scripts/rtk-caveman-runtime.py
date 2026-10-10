#!/usr/bin/env python3

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile


FAILURE = b"FAIL tests/payment_test.go:42 expected 200 got 503"
RTK_HANDLE = re.compile(rb"rtk recall ([0-9a-f]+)")


def fixture(mode):
    lines = [f"PASS tests/module_{i:03d}.go: success\n" for i in range(700)]
    lines.append(FAILURE.decode() + "\n")
    if mode == "repeated":
        lines.extend("PASS tests/boring.go: success " + "x" * 1000 + "\n" for _ in range(4))
    else:
        lines.append("exit status 7\n")
    return "".join(lines).encode()


def cargo_fixture():
    lines = [f"test tests::module_{i:03d} ... ok\n" for i in range(700)]
    lines.extend([
        "test tests::payment ... FAILED\n",
        "\nfailures:\n",
        "---- tests::payment stdout ----\n",
        "thread panicked at tests/payment.rs:42: expected 200, got 503\n",
        "\nfailures:\n",
        "    tests::payment\n",
        "test result: FAILED. 700 passed; 1 failed; 0 ignored\n",
    ])
    return "".join(lines).encode()


def emit_test(counter, mode):
    path = Path(counter)
    count = int(path.read_text()) if path.exists() else 0
    path.write_text(str(count + 1))
    sys.stdout.buffer.write(fixture(mode))
    return 7


def run(argv, env, data=None):
    return subprocess.run(argv, input=data, capture_output=True, env=env, timeout=60, check=False)


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def json_report(stderr):
    for line in stderr.splitlines():
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            return value
    return {}


def base_env(root, engine):
    env = {
        "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
        "XDG_CONFIG_HOME": str(root / "config"),
        "XDG_DATA_HOME": str(root / "data"),
        "RTK_RECALL_DB": str(root / "rtk-recall.db"),
        "RTK_DB_PATH": str(root / "rtk-history.db"),
        "RTK_TELEMETRY_DISABLED": "1",
        "CAVEMAN_HOME": str(root / "caveman"),
        "CAVEMAN_CCR_DB": str(root / "caveman-ccr.db"),
        "CAVEMAN_ENGINE_BIN": str(engine),
        "CAVEMAN_TELEMETRY": "0",
        "DO_NOT_TRACK": "1",
    }
    for name in ("HOME", "LANG", "LC_ALL", "LC_CTYPE"):
        if name in os.environ:
            env[name] = os.environ[name]
    return env


def rtk_case(rtk, env, root, mode):
    counter = root / (mode + "-runs")
    command = [str(rtk), "test", sys.executable, str(Path(__file__).resolve()), "emit-test", str(counter), mode]
    result = run(command, env)
    require(result.returncode == 7, f"RTK {mode}: exit {result.returncode}, want 7")
    require(counter.read_text() == "1", f"RTK {mode}: command ran more than once")
    require(FAILURE in result.stdout, f"RTK {mode}: decisive error or path missing")
    match = RTK_HANDLE.search(result.stdout)
    require(match is not None, f"RTK {mode}: missing recall hint")
    handle = match.group(1).decode()
    recovered = run([str(rtk), "recall", handle, "--full"], env)
    original = fixture(mode)
    require(recovered.returncode == 0 and recovered.stdout == original, f"RTK {mode}: recall differs from original")
    require(counter.read_text() == "1", f"RTK {mode}: recall reran command")
    return result.stdout, handle, {
        "command_exit": result.returncode,
        "command_runs": int(counter.read_text()),
        "original_bytes": len(original),
        "original_sha256": sha(original),
        "rtk_bytes": len(result.stdout),
        "rtk_sha256": sha(result.stdout),
        "error_and_path_visible": True,
        "rtk_recall_exact_original": True,
    }


def caveman_compress(cli, env, data):
    result = run(cli + ["compress", "--type", "terminal"], env, data)
    require(result.returncode == 0, f"Caveman compression failed: {result.stderr.decode(errors='replace')}")
    return result.stdout, json_report(result.stderr)


def experiment(rtk, cli, engine):
    with tempfile.TemporaryDirectory(prefix="agnostic-ai-1960-") as directory:
        root = Path(directory)
        env = base_env(root, engine)
        mode = run([str(rtk), "config", "recall"], env)
        require(mode.returncode == 0 and b"recall mode: sqlite" in mode.stdout, "RTK recall must already use sqlite; this runner does not change its configuration")
        normal, normal_handle, normal_data = rtk_case(rtk, env, root, "normal")
        normal_compact, normal_report = caveman_compress(cli, env, normal)
        require(normal_compact == normal and "recovery_handle" not in normal_report, "normal RTK summary unexpectedly transformed")
        normal_data["caveman_bytes"] = len(normal_compact)
        normal_data["caveman_noop"] = True

        repeated, repeated_handle, repeated_data = rtk_case(rtk, env, root, "repeated")
        compact, report = caveman_compress(cli, env, repeated)
        ccr_handle = report.get("recovery_handle")
        require(ccr_handle and len(compact) < len(repeated), "Caveman did not reduce long RTK summary")
        require(FAILURE in compact and b"rtk recall " + repeated_handle.encode() in compact,
                "Caveman removed decisive error, path, or RTK recall hint")
        recovered = run(cli + ["retrieve", ccr_handle], env)
        require(recovered.returncode == 0 and recovered.stdout == repeated, "Caveman did not recover exact RTK summary")
        require(recovered.stdout != fixture("repeated"), "Caveman unexpectedly recovered pre-RTK input")
        repeated_data.update({
            "caveman_bytes": len(compact),
            "caveman_sha256": sha(compact),
            "caveman_recall_exact_rtk_summary": True,
            "caveman_recall_is_not_original": True,
            "rtk_recall_hint_survives_caveman": True,
            "caveman_report_basis": report.get("basis"),
        })

        wrong_ccr = dict(env, CAVEMAN_CCR_DB=str(root / "wrong-ccr.db"))
        wrong_ccr_result = run(cli + ["retrieve", ccr_handle], wrong_ccr)
        require(wrong_ccr_result.returncode != 0 and wrong_ccr_result.stdout != repeated,
                "wrong Caveman store unexpectedly recovered data")
        wrong_rtk = dict(env, RTK_RECALL_DB=str(root / "wrong-rtk.db"))
        wrong_rtk_result = run([str(rtk), "recall", repeated_handle, "--full"], wrong_rtk)
        require(wrong_rtk_result.returncode != 0 and wrong_rtk_result.stdout != fixture("repeated"),
                "wrong RTK store unexpectedly recovered data")
        missing_engine, missing_engine_report = caveman_compress(
            cli, dict(env, CAVEMAN_ENGINE_BIN=str(root / "missing-engine")), repeated)
        require(missing_engine == repeated and "recovery_handle" not in missing_engine_report,
                "missing engine did not pass through")
        unavailable_ccr = run(cli + ["compress", "--type", "terminal"],
                              dict(env, CAVEMAN_CCR_DB=str(root / "missing-parent" / "ccr.db")), repeated)
        require(unavailable_ccr.returncode != 0 and not unavailable_ccr.stdout,
                "unavailable recovery store produced a misleading compact result")
        require(b"cannot open recovery store" in unavailable_ccr.stderr,
                "unavailable recovery store lacked a clear error")
        short, short_report = caveman_compress(cli, env, b"hello\n")
        require(short == b"hello\n" and "recovery_handle" not in short_report,
                "short input did not pass through")
        repeated_data["command_runs_after_all_recovery"] = int((root / "repeated-runs").read_text())
        require(repeated_data["command_runs_after_all_recovery"] == 1, "a recovery path reran the command")

        disabled_root = root / "disabled-rtk"
        disabled_root.mkdir()
        disabled_env = dict(base_env(disabled_root, engine), RTK_RECALL="0")
        disabled_counter = disabled_root / "disabled-runs"
        disabled = run([str(rtk), "test", sys.executable, str(Path(__file__).resolve()),
                        "emit-test", str(disabled_counter), "normal"], disabled_env)
        require(disabled.returncode == 7 and disabled_counter.read_text() == "1", "disabled RTK command failed")
        require(RTK_HANDLE.search(disabled.stdout) is None, "disabled RTK store still advertised recall")
        require(len(disabled.stdout) < len(fixture("normal")), "disabled RTK store did not filter output")

        cargo_raw = cargo_fixture()
        piped = run([str(rtk), "pipe", "--filter", "cargo-test"], env, cargo_raw)
        require(piped.returncode == 0 and len(piped.stdout) < len(cargo_raw), "RTK pipe did not filter fixture")
        require(b"tests/payment.rs:42" in piped.stdout, "RTK pipe hid the error path")
        require(RTK_HANDLE.search(piped.stdout) is None, "RTK pipe unexpectedly offered recall")

        wrapped_counter = root / "wrapped-runs"
        wrapped = run(cli + ["shrink", "--", str(rtk), "test", sys.executable,
                             str(Path(__file__).resolve()), "emit-test", str(wrapped_counter), "normal"], env)
        require(wrapped.returncode == 7 and wrapped_counter.read_text() == "1", "combined wrapper lost exit 7 or reran command")
        require(FAILURE in wrapped.stdout, "combined wrapper hid decisive error or path")
        long_wrapped_counter = root / "long-wrapped-runs"
        long_wrapped = run(cli + ["shrink", "--", str(rtk), "test", sys.executable,
                                  str(Path(__file__).resolve()), "emit-test", str(long_wrapped_counter), "repeated"], env)
        long_wrapped_report = json_report(long_wrapped.stderr)
        long_wrapped_handle = long_wrapped_report.get("recovery_handle")
        require(long_wrapped.returncode == 7 and long_wrapped_counter.read_text() == "1",
                "long nested wrapper lost exit 7 or reran command")
        require(long_wrapped_handle and long_wrapped.stdout.startswith(compact),
                "long nested wrapper did not include the direct Caveman transform")
        require(long_wrapped_handle.encode() in long_wrapped.stdout,
                "long nested wrapper omitted the Caveman recovery hint")
        long_wrapped_recovery = run(cli + ["retrieve", long_wrapped_handle], env)
        require(long_wrapped_recovery.returncode == 0 and long_wrapped_recovery.stdout == repeated,
                "long nested wrapper did not recover the exact RTK summary")
        require(long_wrapped_counter.read_text() == "1", "long nested recovery reran command")
        unavailable_counter = root / "unavailable-wrapper-runs"
        unavailable_wrapper = run(cli + ["shrink", "--", str(rtk), "test", sys.executable,
                                         str(Path(__file__).resolve()), "emit-test", str(unavailable_counter), "normal"],
                                  dict(env, CAVEMAN_CCR_DB=str(root / "missing-parent" / "ccr.db")))
        require(unavailable_wrapper.returncode == 7 and unavailable_counter.read_text() == "1",
                "unavailable recovery wrapper changed exit status or reran command")
        require(unavailable_wrapper.stdout == normal, "unavailable recovery wrapper did not pass through the exact RTK summary")
        missing_rtk_counter = root / "missing-rtk-runs"
        missing_rtk = run(cli + ["shrink", "--", str(root / "missing-rtk"), "test", sys.executable,
                                 str(Path(__file__).resolve()), "emit-test", str(missing_rtk_counter), "normal"], env)
        require(missing_rtk.returncode != 0 and not missing_rtk_counter.exists(),
                "missing RTK unexpectedly executed the command")

        return {
            "versions": {
                "rtk": run([str(rtk), "--version"], env).stdout.decode().strip(),
                "rtk_sha256": sha(rtk.read_bytes()),
                "caveman_cli": json.loads((Path(cli[1]).parents[1] / "package.json").read_text())["version"],
                "caveman_cli_entry_sha256": sha(Path(cli[1]).read_bytes()),
                "caveman_engine_sha256": sha(engine.read_bytes()),
            },
            "platform": sys.platform,
            "recall_mode": "sqlite with temporary stores",
            "home_inherited": True,
            "rtk_configuration_modified": False,
            "normal": normal_data,
            "long_summary": repeated_data,
            "stdin_filter": {
                "input_bytes": len(cargo_raw),
                "output_bytes": len(piped.stdout),
                "pipe_exit": piped.returncode,
                "error_path_visible": True,
                "rtk_recall_hint": False,
            },
            "failure_cases": {
                "wrong_caveman_store_fails": True,
                "wrong_rtk_store_fails": True,
                "missing_engine_passes_original_rtk_summary": True,
                "unavailable_caveman_store_exit": unavailable_ccr.returncode,
                "unavailable_caveman_store_emits_no_compact_output": True,
                "unavailable_caveman_store_error_identifies_store": True,
                "unavailable_store_wrapper_exit": unavailable_wrapper.returncode,
                "unavailable_store_wrapper_command_runs": int(unavailable_counter.read_text()),
                "unavailable_store_wrapper_error_and_path_visible": True,
                "unavailable_store_wrapper_exact_rtk_summary": True,
                "short_input_noop": True,
                "disabled_rtk_store_has_no_recall_hint": True,
                "disabled_rtk_filtered_bytes": len(disabled.stdout),
                "disabled_rtk_command_runs": int(disabled_counter.read_text()),
                "nested_wrapper_exit": wrapped.returncode,
                "nested_wrapper_command_runs": int(wrapped_counter.read_text()),
                "nested_wrapper_error_and_path_visible": True,
                "long_nested_wrapper_exit": long_wrapped.returncode,
                "long_nested_wrapper_command_runs": int(long_wrapped_counter.read_text()),
                "long_nested_wrapper_bytes_including_status": len(long_wrapped.stdout),
                "long_nested_wrapper_includes_direct_transform": True,
                "long_nested_wrapper_recovers_rtk_summary": True,
                "missing_rtk_wrapper_exit": missing_rtk.returncode,
                "missing_rtk_command_not_run": True,
            },
        }


def main():
    if len(sys.argv) == 4 and sys.argv[1] == "emit-test":
        return emit_test(sys.argv[2], sys.argv[3])
    parser = argparse.ArgumentParser()
    parser.add_argument("--rtk", type=Path, required=True)
    parser.add_argument("--node", type=Path, required=True)
    parser.add_argument("--caveman-cli", type=Path, required=True)
    parser.add_argument("--caveman-engine", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    for path in (args.rtk, args.node, args.caveman_cli, args.caveman_engine):
        require(path.is_file(), f"missing executable or script: {path}")
    result = experiment(args.rtk.resolve(), [str(args.node.resolve()), str(args.caveman_cli.resolve()), "tools"],
                        args.caveman_engine.resolve())
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    print(f"wrote {args.output}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
