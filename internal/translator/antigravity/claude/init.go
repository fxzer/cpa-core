package claude

import (
	. "github.com/fxzer/cpa-core/v7/internal/constant"
	"github.com/fxzer/cpa-core/v7/internal/interfaces"
	"github.com/fxzer/cpa-core/v7/internal/translator/translator"
)

func init() {
	translator.Register(
		Claude,
		Antigravity,
		ConvertClaudeRequestToAntigravity,
		interfaces.TranslateResponse{
			Stream:     ConvertAntigravityResponseToClaude,
			NonStream:  ConvertAntigravityResponseToClaudeNonStream,
			TokenCount: ClaudeTokenCount,
		},
	)
}
