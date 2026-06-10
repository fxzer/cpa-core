package gemini

import (
	. "github.com/fxzer/cpa-core/v7/internal/constant"
	"github.com/fxzer/cpa-core/v7/internal/interfaces"
	"github.com/fxzer/cpa-core/v7/internal/translator/translator"
)

func init() {
	translator.Register(
		Gemini,
		GeminiCLI,
		ConvertGeminiRequestToGeminiCLI,
		interfaces.TranslateResponse{
			Stream:     ConvertGeminiCliResponseToGemini,
			NonStream:  ConvertGeminiCliResponseToGeminiNonStream,
			TokenCount: GeminiTokenCount,
		},
	)
}
