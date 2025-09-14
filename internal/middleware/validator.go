package middleware

import (
	"context"
	"errors"
	"fmt"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
)

// HTTPValidator creates a Connect middleware that validates protobuf messages using protovalidate
func HTTPValidator() connect.UnaryInterceptorFunc {
	validator, err := protovalidate.New()
	if err != nil {
		panic(fmt.Sprintf("failed to create protovalidate validator: %v", err))
	}

	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// 获取请求消息
			msg := req.Any()

			// 检查是否为protobuf消息
			protoMsg, ok := msg.(proto.Message)
			if !ok {
				// 如果不是protobuf消息，跳过验证
				return next(ctx, req)
			}

			// 执行验证
			if err := validator.Validate(protoMsg); err != nil {
				var validationError *protovalidate.ValidationError
				if errors.As(err, &validationError) {
					return nil, connect.NewError(
						connect.CodeInvalidArgument,
						fmt.Errorf("validation failed: %s", formatValidationError(validationError)),
					)
				}
				return nil, connect.NewError(
					connect.CodeInvalidArgument,
					fmt.Errorf("validation failed: %v", err),
				)
			}

			// 验证通过，继续处理请求
			return next(ctx, req)
		}
	}
}

// formatValidationError 格式化验证错误信息
func formatValidationError(validationError *protovalidate.ValidationError) string {
	if len(validationError.Violations) == 0 {
		return "validation failed"
	}

	// 收集所有验证错误
	var messages []string
	for _, violation := range validationError.Violations {
		if violation.Proto != nil {
			message := violation.Proto.GetMessage()
			fieldPath := ""

			// 尝试从Field路径获取字段名
			if field := violation.Proto.GetField(); field != nil && len(field.GetElements()) > 0 {
				// 获取最后一个元素的字段名
				lastElement := field.GetElements()[len(field.GetElements())-1]
				if lastElement.GetFieldName() != "" {
					fieldPath = lastElement.GetFieldName()
				}
			}

			if fieldPath != "" {
				messages = append(messages, fmt.Sprintf("field '%s': %s", fieldPath, message))
			} else {
				messages = append(messages, message)
			}
		} else {
			messages = append(messages, "validation error")
		}
	}

	if len(messages) == 1 {
		return messages[0]
	}

	// 多个错误时返回详细列表
	result := "multiple validation errors:"
	for i, msg := range messages {
		result += fmt.Sprintf("\n  %d. %s", i+1, msg)
	}
	return result
}
