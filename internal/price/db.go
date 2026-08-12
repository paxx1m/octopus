package price

import (
	"context"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
)

// AddModelsToDB 将模型名写入数据库，优先附带预设价格表。
func AddModelsToDB(modelNames []string, ctx context.Context) error {
	newLLMInfos := make([]model.LLMInfo, 0, len(modelNames))
	newLLMNames := make([]string, 0, len(modelNames))
	for _, modelName := range modelNames {
		if modelName == "" {
			continue
		}
		modelPrice := GetLLMPrice(modelName)
		if modelPrice != nil {
			newLLMInfos = append(newLLMInfos, model.LLMInfo{
				Name:     modelName,
				LLMPrice: *modelPrice,
			})
		} else {
			newLLMInfos = append(newLLMInfos, model.LLMInfo{Name: modelName})
		}
		newLLMNames = append(newLLMNames, modelName)
	}
	if len(newLLMInfos) > 0 {
		return op.LLMBatchCreate(newLLMInfos, ctx)
	}
	return nil
}

// DeleteModelsWithNoPrice 删除无价格的模型（价格全零时视为无价格）。
func DeleteModelsWithNoPrice(modelNames []string, ctx context.Context) error {
	if len(modelNames) == 0 {
		return nil
	}
	needDeleteModelNames := make([]string, 0, len(modelNames))
	for _, modelName := range modelNames {
		if modelName == "" {
			continue
		}
		modelPrice, err := op.LLMGet(modelName)
		if err != nil {
			return err
		}
		if modelPrice.Input != 0 || modelPrice.Output != 0 || modelPrice.CacheRead != 0 || modelPrice.CacheWrite != 0 {
			continue
		}
		needDeleteModelNames = append(needDeleteModelNames, modelName)
	}
	if len(needDeleteModelNames) > 0 {
		return op.LLMBatchDelete(needDeleteModelNames, ctx)
	}
	return nil
}
