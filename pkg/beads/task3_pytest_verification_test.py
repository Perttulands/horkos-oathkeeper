import subprocess


def test_task3_go_package_passes():
    result = subprocess.run(
        ["/usr/local/go/bin/go", "test", "./pkg/beads/...", "-count=1"],
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, result.stdout + "\n" + result.stderr
