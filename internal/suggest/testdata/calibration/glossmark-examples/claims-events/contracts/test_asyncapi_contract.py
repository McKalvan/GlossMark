"""Contract test: every AsyncAPI message points at a registered Avro schema whose
record name matches, and every message on a channel has a registry subject. Runs in CI before any service is built."""
import json
from pathlib import Path

import pytest
import yaml

ROOT = Path(__file__).resolve().parents[1]
SPEC = yaml.safe_load((ROOT / "contracts" / "asyncapi.yaml").read_text())
SUBJECTS = yaml.safe_load((ROOT / "schemas" / "subjects.yaml").read_text())["subjects"]


def _schema(message: dict) -> dict:
    ref = message["payload"]["schema"]["$ref"]
    return json.loads((ROOT / "contracts" / ref).resolve().read_text())


@pytest.mark.parametrize("name,message", sorted(SPEC["components"]["messages"].items()))
def test_message_payload_matches_record_name(name, message):
    assert _schema(message)["name"] == name


def test_every_channel_message_has_a_subject():
    on_channels = set()
    for channel in SPEC["channels"].values():
        for msg_ref in channel["messages"].values():
            name = msg_ref["$ref"].rsplit("/", 1)[-1]
            on_channels.add(f"{channel['address']}-com.tidewater.claims.events.{name}")
    missing = on_channels - set(SUBJECTS)
    assert not missing, f"channel messages with no registry subject: {sorted(missing)}"


# @gloss verifies claim:ClaimFiled conf=0.9
def test_filed_events_share_the_claim_ref():
    for name in ("MotorClaimFiled", "FloodClaimFiled", "FireClaimFiled"):
        fields = {f["name"]: f for f in _schema(SPEC["components"]["messages"][name])["fields"]}
        assert fields["claim"]["type"] == "com.tidewater.claims.common.ClaimRef"
