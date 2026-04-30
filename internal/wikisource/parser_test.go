package wikisource

import "testing"

func TestParseMapsSourceLinksToTraceMentions(t *testing.T) {
	parsed := Parse(".docs/wiki/10_contratos/CT-AI-GATEWAY-FORMATS.md", `# CT-AI-GATEWAY-FORMATS

wiki_source_protocol: SDD-WIKI-SOURCE-v1
doc_id: CT-AI-GATEWAY-FORMATS
code_links:
  - src/backend/MultiTedi.Contracts/InternalApi/AI/AiContracts.cs
test_links:
  - src/backend/tests/MultiTedi.ControlPlane.Tests/Services/GlobalJudgeValidatorTests.cs

`+"```toon"+`
block_id: CT-AI-GATEWAY-FORMATS.global-judge
kind: contract
source_of_truth: CT-AI-GATEWAY-FORMATS
code_links:
  - src/backend/MultiTedi.ControlPlane.Application/Services/AiGatewayApplicationService.cs
test_links:
  - src/backend/runtime/orchestrator/tests/test_runtime_service.py
`+"```"+`
`, 1)

	assertMention(t, parsed, "implements", "src/backend/MultiTedi.Contracts/InternalApi/AI/AiContracts.cs")
	assertMention(t, parsed, "test_file", "src/backend/tests/MultiTedi.ControlPlane.Tests/Services/GlobalJudgeValidatorTests.cs")
	assertMention(t, parsed, "implements", "src/backend/MultiTedi.ControlPlane.Application/Services/AiGatewayApplicationService.cs")
	assertMention(t, parsed, "test_file", "src/backend/runtime/orchestrator/tests/test_runtime_service.py")
}

func assertMention(t *testing.T, parsed ParsedDoc, kind string, value string) {
	t.Helper()
	for _, mention := range parsed.Mentions {
		if mention.MentionType == kind && mention.MentionValue == value {
			return
		}
	}
	t.Fatalf("missing mention %s=%s in %#v", kind, value, parsed.Mentions)
}
