package responses

import (
	. "github.com/fxzer/cpa-core/v7/internal/constant"
	"github.com/fxzer/cpa-core/v7/internal/interfaces"
	"github.com/fxzer/cpa-core/v7/internal/translator/translator"
)

func init() {
	translator.Register(
		OpenaiResponse,
		Gemini,
		ConvertOpenAIResponsesRequestToGemini,
		interfaces.TranslateResponse{
			Stream:    ConvertGeminiResponseToOpenAIResponses,
			NonStream: ConvertGeminiResponseToOpenAIResponsesNonStream,
		},
	)
}
