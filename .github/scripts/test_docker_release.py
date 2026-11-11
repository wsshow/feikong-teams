"""覆盖预发布、补发旧版及不可信 tag 的发布边界。"""

import unittest

from docker_release import parse_tag, should_update_latest


class DockerReleaseTests(unittest.TestCase):
    def test_stable_and_prerelease_tags(self):
        self.assertEqual(parse_tag("v0.30.0"), ((0, 30, 0), None))
        self.assertEqual(parse_tag("v1.2.3-rc.1"), ((1, 2, 3), "rc.1"))

    def test_rejects_invalid_or_unsafe_tags(self):
        for tag in ["1.2.3", "v01.2.3", "v1.2.3-01", "v1.2.3+build",
                    "v1.2.3\nlatest=true", "v1.2.3;id", "v1.2.3-$(id)",
                    "v1.2.3/other", "v1.2.3-" + "a" * 128]:
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                parse_tag(tag)

    def test_prerelease_never_updates_latest(self):
        self.assertFalse(should_update_latest("v2.0.0-rc.1", ["v1.0.0"]))

    def test_old_release_does_not_roll_back_latest(self):
        self.assertFalse(should_update_latest("v0.9.9", ["v0.10.0", "v0.9.9"]))
        self.assertFalse(should_update_latest("v1.99.0", ["v2.0.0"]))

    def test_latest_stable_can_be_retried(self):
        self.assertTrue(should_update_latest("v0.30.0", ["v0.30.0", "v0.29.1"]))

    def test_future_prerelease_and_unrelated_tags_do_not_block_stable(self):
        self.assertTrue(should_update_latest("v1.0.0", ["v2.0.0-rc.1", "archive", "vbad"]))
        self.assertTrue(should_update_latest("v1.0.0", []))


if __name__ == "__main__":
    unittest.main()
