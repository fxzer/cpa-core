package geminiCLI

import (
	. "github.com/fxzer/cpa-core/v7/internal/constant"
	"github.com/fxzer/cpa-core/v7/internal/interfaces"
	"github.com/fxzer/cpa-core/v7/internal/translator/translator"
)

func init() {
	translator.Register(
		GeminiCLI,
		Codex,
		ConvertGeminiCLIRequestToCodex,
		interfaces.TranslateResponse{
			Stream:     ConvertCodexResponseToGeminiCLI,
			NonStream:  ConvertCodexResponseToGeminiCLINonStream,
			TokenCount: GeminiCLITokenCount,
		},
	)
}
