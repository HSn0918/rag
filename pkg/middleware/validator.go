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
			msg := req.Any()
			protoMsg, ok := msg.(proto.Message)
			if !ok {
				return next(ctx, req)
			}
			if err := validator.Validate(protoMsg); err != nil {
				var validationError *protovalidate.ValidationError
				if errors.As(err, &validationError) {
					return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("validation failed: %s", formatValidationError(validationError)))
				}
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("validation failed: %v", err))
			}
			return next(ctx, req)
		}
	}
}

func formatValidationError(validationError *protovalidate.ValidationError) string {
	if len(validationError.Violations) == 0 {
		return "validation failed"
	}
	var messages []string
	for _, violation := range validationError.Violations {
		if violation.Proto != nil {
			message := violation.Proto.GetMessage()
			fieldPath := ""
			if field := violation.Proto.GetField(); field != nil && len(field.GetElements()) > 0 {
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
	result := "multiple validation errors:"
	for i, msg := range messages {
		result += fmt.Sprintf("\n  %d. %s", i+1, msg)
	}
	return result
}
