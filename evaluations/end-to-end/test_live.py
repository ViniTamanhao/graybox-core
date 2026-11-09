"""No inference: exercise artifact retention and isolation across namespace exit."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from live import launch_args, isolation


@unittest.skipUnless(shutil.which('bwrap'), 'Linux bubblewrap required')
class IsolationTests(unittest.TestCase):
    def test_private_report_survives_namespace_exit(self):
        with tempfile.TemporaryDirectory(prefix='commerce-isolation-') as tmp:
            trial=Path(tmp)/'trial';trial.mkdir(mode=0o700)
            (trial/'app').mkdir(mode=0o700)
            state=trial/'home/.codex';state.mkdir(parents=True,mode=0o700)
            # No real inference/authentication is needed by this filesystem check.
            (state/'auth.json').write_text('{}')
            env={'API_AUTH':'synthetic-a','OTHER_AUTH':'synthetic-b'}
            self.assertEqual(isolation(trial,env),'isolated')
            code="import os,pathlib; os.umask(0o077); pathlib.Path('/tmp/private-report').write_text('evidence')"
            result=subprocess.run(launch_args(trial,env)+['python','-c',code],capture_output=True,text=True,timeout=30)
            self.assertEqual(result.returncode,0,result.stderr)
            report=trial/'scratch/private-report'
            self.assertTrue(report.exists(),'agent report disappeared at namespace teardown')
            self.assertEqual(report.read_text(),'evidence')
            self.assertEqual(report.stat().st_mode & 0o777,0o600)
            self.assertEqual((trial/'scratch').stat().st_mode & 0o777,0o700)

if __name__=='__main__':unittest.main()
