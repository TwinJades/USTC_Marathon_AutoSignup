import unittest
import uuid
from pydantic import ValidationError

from ustc_marathon.config import ROOT, RUNTIME, Config, Monitor, Profile, Target, read_config, write_setting
from ustc_marathon.errors import Problem
from ustc_marathon.state import Journal, account_key, single_instance
from ustc_marathon.website import required_integer


class ConfigurationChecks(unittest.TestCase):
    def test_shipped_example_is_valid_and_private_data_is_empty(self):
        config = read_config(ROOT / "config.example.toml")
        self.assertEqual(config.monitor.poll_interval_seconds, 30)
        self.assertEqual(config.targets, [])
        self.assertEqual(config.profile.phone, "")
        self.assertEqual(config.monitor.username, "")
        with self.assertRaises(Problem):
            config.require_watch()

    def test_intervals_are_customizable_and_finite(self):
        for interval in (0.25, 10, 30, 123.456):
            self.assertEqual(Monitor(poll_interval_seconds=interval).poll_interval_seconds, interval)
        for interval in (0, -1, float("inf"), float("nan")):
            with self.assertRaises(ValidationError):
                Monitor(poll_interval_seconds=interval)

    def test_missing_profile_stops_before_any_network_operation(self):
        with self.assertRaises(Problem):
            Profile().require_complete()

    def test_unknown_configuration_fields_are_rejected(self):
        with self.assertRaises(ValidationError):
            Config.model_validate({"profile": {"password": ""}})

    def test_toml_library_preserves_target_order_and_comments(self):
        root = RUNTIME / "local-verification" / uuid.uuid4().hex
        root.mkdir(parents=True)
        path = root / "config.toml"
        path.write_text((ROOT / "config.example.toml").read_text(encoding="utf-8"), encoding="utf-8")
        targets = [Target(id="local-validation-a", name="本地配置验证甲", campus="西校区"),
                   Target(id="local-validation-b", name="本地配置验证乙", campus="高新校区")]
        write_setting("", "targets", [target.model_dump() for target in targets], path)
        self.assertEqual(read_config(path).targets, targets)
        self.assertIn("两个步骤共用", path.read_text(encoding="utf-8"))
        write_setting("monitor", "poll_interval_seconds", 2.5, path)
        self.assertEqual(read_config(path).monitor.poll_interval_seconds, 2.5)


class PersistenceChecks(unittest.TestCase):
    def test_database_survives_restart_and_blocks_other_accounts(self):
        root = RUNTIME / "local-verification" / uuid.uuid4().hex
        target = Target(id="local-database-validation", name="本地数据库事务验证", campus="西校区")
        journal = Journal(root)
        journal.record("local-validation-account", target, "enter_pending")
        journal.close()
        reopened = Journal(root)
        try:
            self.assertEqual(reopened.unresolved("local-validation-account")[0]["phase"], "enter_pending")
            with self.assertRaises(Problem):
                reopened.unresolved("different-local-account")
            reopened.record("local-validation-account", target, "reserved")
            reopened.record("local-validation-account", target, "save_pending")
            reopened.record("local-validation-account", target, "complete")
            self.assertEqual(reopened.unresolved("local-validation-account"), [])
            self.assertEqual(len(reopened.rows()), 1)
            log = reopened.log_path.read_text(encoding="utf-8")
            self.assertNotIn("local-validation-account", log)
        finally:
            reopened.close()

    def test_single_instance_uses_actual_windows_file_lock(self):
        root = RUNTIME / "local-verification" / uuid.uuid4().hex
        with single_instance(root):
            with self.assertRaises(OSError):
                with single_instance(root):
                    self.fail("第二个实例取得了同一锁定文件")
        with single_instance(root):
            pass

    def test_account_identifier_is_hashed(self):
        self.assertEqual(len(account_key("local-validation-account")), 64)
        self.assertNotEqual(account_key("local-validation-account"), account_key("another-local-account"))


class InputBoundaryChecks(unittest.TestCase):
    def test_unknown_occupancy_cannot_be_interpreted_as_available(self):
        for value in (None, True, False, -1, 1.5, "unknown", ""):
            with self.assertRaises(Problem):
                required_integer({"registrationNum": value}, "registrationNum")
        self.assertEqual(required_integer({"registrationNum": "12"}, "registrationNum"), 12)

    def test_submission_target_cannot_include_url_characters(self):
        with self.assertRaises(ValidationError):
            Target(id="../other", name="本地字段验证", campus="西校区")


if __name__ == "__main__":
    unittest.main()
