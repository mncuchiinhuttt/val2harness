"""Run against the opt-in local Go fixture; requires anthropic==2.3.0.

No actual account, external URL fetch, or tool outside the fixture is used.
"""
import json
import sys
from pathlib import Path
import anthropic

root = Path(sys.argv[1])
gateway = json.loads((root / "gateway.json").read_text())
client = anthropic.Anthropic(base_url=gateway["url"], api_key=gateway["key"], max_retries=0,
                             timeout=20, _strict_response_validation=True,
                             http_client=anthropic.DefaultHttpxClient(verify=False, trust_env=False))
tools = [dict(name=name, description="Inspect synthetic data", input_schema={"type":"object", "properties":{}})
         for name in ("Read", "Inspect")]
png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a8ncAAAAASUVORK5CYII="
for streaming in (False, True):
    messages = [{"role":"user", "content":[{"type":"text","text":"Read the local synthetic fixture using both tools."},
                 {"type":"image","source":{"type":"base64","media_type":"image/png","data":png}}]}]
    options = dict(model="synthetic-sdk-tools", max_tokens=64, messages=messages, tools=tools)
    if streaming:
        with client.messages.stream(**options) as stream:
            first = stream.get_final_message()
    else:
        first = client.messages.create(**options)
    calls = [block for block in first.content if block.type == "tool_use"]
    assert first.stop_reason == "tool_use" and [call.name for call in calls] == ["Read", "Inspect"]
    assert first.usage.input_tokens == 30 and first.usage.output_tokens == 12
    results = []
    for call in calls:
        if call.name == "Read":
            fixture = Path(call.input["file_path"]).resolve()
            assert fixture == (root / "fixture.txt").resolve()
            value = fixture.read_text().strip()
        else:
            value = "synthetic inspected"
        results.append({"type":"tool_result", "tool_use_id":call.id, "content":[{"type":"text","text":value},
                        {"type":"image","source":{"type":"url","url":"https://example.invalid/result.png"}}],
                        "is_error":call.name == "Inspect"})
    messages.extend([{"role":"assistant", "content":[b.model_dump(exclude_none=True) for b in first.content]},
                     {"role":"user", "content":results}])
    if streaming:
        with client.messages.stream(**options) as stream:
            final = stream.get_final_message()
    else:
        final = client.messages.create(**options)
    assert final.stop_reason == "end_turn"
    assert "fixture-token-519" in "".join(b.text for b in final.content if b.type == "text")
    print("PASS:", "stream" if streaming else "JSON", "parallel tools, SDK accumulation, real results and image inputs")
print("Anthropic SDK", anthropic.__version__)
client.close()
