from s3logsbeat import BaseTest

import os


class Test(BaseTest):

    def test_base(self):
        """
        Basic test with exiting S3logsbeat normally
        """
        self.render_config_template(
            path=os.path.abspath(self.working_dir) + "/log/*"
        )

        s3logsbeat_proc = self.start_beat()
        self.wait_until(lambda: self.log_contains("s3logsbeat is running"))
        exit_code = s3logsbeat_proc.kill_and_wait()
        assert exit_code == 0
