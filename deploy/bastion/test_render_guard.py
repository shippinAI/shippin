import unittest
from render_guard import render


class GuardConfigTests(unittest.TestCase):
    def setUp(self):
        self.config = {"edge_ipv4": "192.0.2.10", "edge_interface": "eth0",
                       "ssh_interfaces": ["lo", "tailscale0"], "ssh_port": 22,
                       "terminal_port": 7681}

    def test_rejects_injected_or_broad_network_inputs(self):
        for field, value in [("edge_ipv4", "0.0.0.0/0"), ("edge_ipv4", "0.0.0.0"),
                             ("edge_ipv4", "192.0.2.10; flush ruleset"),
                             ("edge_interface", 'eth0" accept'),
                             ("ssh_interfaces", ["eth0"]), ("ssh_interfaces", ["*"]),
                             ("terminal_port", "7681; flush ruleset"),
                             ("terminal_port", 22), ("ssh_port", True)]:
            with self.subTest(field=field, value=value), self.assertRaises((ValueError, TypeError)):
                render({**self.config, field: value})

    def test_terminal_rejection_covers_existing_and_ipv6_connections(self):
        rules = render(self.config)["shippin-bastion.nft"]
        self.assertIn("table inet", rules)
        self.assertNotIn("ct state", rules)
        self.assertIn("tcp dport 7681 counter reject with tcp reset", rules)
        self.assertNotIn("flush ruleset", rules)

    def test_services_require_guard_at_startup(self):
        files = render(self.config)
        self.assertIn("Requires=shippin-bastion-guard.service", files["10-shippin-bastion-guard.conf"])
        self.assertIn("Before=ssh.service bastion-web.service", files["shippin-bastion-guard.service"])


if __name__ == "__main__":
    unittest.main()
