import subprocess


def test_task2_go_packages_pass():
    result = subprocess.run(
        ["/usr/local/go/bin/go", "test", "./pkg/config/...", "./pkg/storage/...", "-count=1"],
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, result.stdout + "\n" + result.stderr
