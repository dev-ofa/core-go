package datax

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrors(t *testing.T) {
	root := errors.New("root cause")

	expected := NewError(ErrCodeExpected, "expected failure", root)
	if got, want := expected.Error(), "expected failure: root cause"; got != want {
		t.Fatalf("expected error message = %q, want %q", got, want)
	}
	if !IsExpected(expected) {
		t.Fatal("expected error should be classified as expected")
	}
	if IsUnexpected(expected) {
		t.Fatal("expected error should not be classified as unexpected")
	}
	if !errors.Is(expected, root) {
		t.Fatal("expected error should unwrap its cause")
	}

	wrappedExpected := fmt.Errorf("wrap: %w", expected)
	if !IsExpected(wrappedExpected) {
		t.Fatal("wrapped expected error should be classified by code")
	}

	unexpected := NewError(ErrCodeUnexpected, "unexpected failure", root)
	if got, want := unexpected.Error(), "unexpected failure: root cause"; got != want {
		t.Fatalf("unexpected error message = %q, want %q", got, want)
	}
	if !IsUnexpected(unexpected) {
		t.Fatal("unexpected error should be classified as unexpected")
	}
	if IsExpected(unexpected) {
		t.Fatal("unexpected error should not be classified as expected")
	}
	if !errors.Is(unexpected, root) {
		t.Fatal("unexpected error should unwrap its cause")
	}
}

func TestUnclassifiedErrorsAreUnexpected(t *testing.T) {
	if !IsUnexpected(errors.New("plain")) {
		t.Fatal("plain error should be unexpected")
	}
}

func TestCommonExpectedErrors(t *testing.T) {
	root := errors.New("root cause")
	item := ValidateErrItem{ParamName: "tenant_id", Reason: "missing"}
	validationErr := NewValidationError("bad request", []ValidateErrItem{item}, root)

	if !IsExpected(validationErr) {
		t.Fatal("validation error should be expected")
	}
	if got, want := validationErr.Error(), "bad request: root cause"; got != want {
		t.Fatalf("validation error message = %q, want %q", got, want)
	}
	if !errors.Is(validationErr, root) {
		t.Fatal("validation error should unwrap its cause")
	}
	if len(validationErr.Items) != 1 || validationErr.Items[0] != item {
		t.Fatalf("validation items = %+v, want %+v", validationErr.Items, []ValidateErrItem{item})
	}

	notFoundErr := NewResourceNotFoundError("users/u1", root)
	if !IsExpected(notFoundErr) {
		t.Fatal("resource not found error should be expected")
	}
	if got, want := notFoundErr.Error(), "resource not found: users/u1: root cause"; got != want {
		t.Fatalf("not found error message = %q, want %q", got, want)
	}
	if notFoundErr.Resource != "users/u1" {
		t.Fatalf("not found resource = %q, want users/u1", notFoundErr.Resource)
	}
	var resourceErr *ResourceError
	if !errors.As(notFoundErr, &resourceErr) {
		t.Fatal("not found error should be ResourceError")
	}

	conflictErr := NewResourceConflictError("users/u1", root)
	if !IsExpected(conflictErr) {
		t.Fatal("resource conflict error should be expected")
	}
	if got, want := conflictErr.Error(), "resource conflict: users/u1: root cause"; got != want {
		t.Fatalf("conflict error message = %q, want %q", got, want)
	}
}

func TestCompatibilityConstructorsReturnCommonErrors(t *testing.T) {
	validateErr := NewValidateError("", nil)
	if !IsExpected(validateErr) {
		t.Fatal("NewValidateError should now return an expected validation error")
	}
	var typedValidation *ValidationError
	if !errors.As(validateErr, &typedValidation) {
		t.Fatal("NewValidateError should return ValidationError")
	}

	internalErr := NewInternalError("")
	if !IsUnexpected(internalErr) {
		t.Fatal("NewInternalError should now return an unexpected internal error")
	}
}

func TestCommonUnexpectedErrors(t *testing.T) {
	root := errors.New("root cause")

	internalErr := NewInternalFailure("state corrupted", root)
	if !IsUnexpected(internalErr) {
		t.Fatal("internal error should be unexpected")
	}
	if got, want := internalErr.Error(), "state corrupted: root cause"; got != want {
		t.Fatalf("internal error message = %q, want %q", got, want)
	}
	if !errors.Is(internalErr, root) {
		t.Fatal("internal error should unwrap its cause")
	}

	upstreamErr := NewUpstreamError("order-service", "GET /v1/orders/o1", "req-1", root)
	if !IsUnexpected(upstreamErr) {
		t.Fatal("upstream error with an unknown cause should be unexpected")
	}
	if got, want := upstreamErr.Error(), "upstream call failed: GET /v1/orders/o1 order-service: root cause"; got != want {
		t.Fatalf("upstream error message = %q, want %q", got, want)
	}
	if !errors.Is(upstreamErr, root) {
		t.Fatal("upstream error should unwrap its cause")
	}
	if upstreamErr.Target != "order-service" || upstreamErr.Operation != "GET /v1/orders/o1" || upstreamErr.RequestID != "req-1" {
		t.Fatalf("upstream context = %+v", upstreamErr)
	}

	upstreamValidationErr := NewUpstreamError("order-service", "POST /v1/orders", "req-2", NewValidationError("bad request", nil, nil))
	if got := CodeOf(upstreamValidationErr); got != ErrCodeValidate {
		t.Fatalf("upstream container code = %d, want cause code %d", got, ErrCodeValidate)
	}
	if !IsExpected(upstreamValidationErr) {
		t.Fatal("upstream container should keep cause classification")
	}
}

func TestCodeOfMapsCommonErrors(t *testing.T) {
	item := ValidateErrItem{ParamName: "tenant_id", Reason: "missing"}
	validationErr := NewValidationError("bad request", []ValidateErrItem{item}, nil)

	if got := CodeOf(validationErr); got != ErrCodeValidate {
		t.Fatalf("validation code = %d, want %d", got, ErrCodeValidate)
	}
	if !IsErrCode(ErrCodeValidate, validationErr) {
		t.Fatal("IsErrCode should match validation errors")
	}

	notFoundErr := NewResourceNotFoundError("users/u1", nil)
	if got := CodeOf(notFoundErr); got != ErrCodeNotFound {
		t.Fatalf("not found code = %d, want %d", got, ErrCodeNotFound)
	}
	if !IsErrCode(ErrCodeNotFound, notFoundErr) {
		t.Fatal("IsErrCode should match not-found errors")
	}

	conflictErr := NewResourceConflictError("users/u1", nil)
	if got := CodeOf(conflictErr); got != ErrCodeConflict {
		t.Fatalf("conflict code = %d, want %d", got, ErrCodeConflict)
	}

	expectedErr := NewError(ErrCodeExpected, "can show this", nil)
	if got := CodeOf(expectedErr); got != ErrCodeExpected {
		t.Fatalf("generic expected code = %d, want %d", got, ErrCodeExpected)
	}

	internalErr := NewInternalFailure("state corrupted", nil)
	if got := CodeOf(internalErr); got != ErrCodeUnexpected {
		t.Fatalf("internal code = %d, want %d", got, ErrCodeUnexpected)
	}
	if errors.Is(internalErr, errors.New("plain")) {
		t.Fatal("internal error should not match an unrelated plain error")
	}

	plainErr := errors.New("plain")
	if got := CodeOf(plainErr); got != ErrCodeUnexpected {
		t.Fatalf("plain code = %d, want %d", got, ErrCodeUnexpected)
	}
}

func TestExtraDataError(t *testing.T) {
	root := NewValidationError("bad request", nil, nil)
	withData := WithErrorData(root, map[string]string{"field": "tenant_id"})

	if !errors.Is(withData, root) {
		t.Fatal("extra data error should unwrap its cause")
	}
	if got := withData.Error(); got != "bad request" {
		t.Fatalf("extra data error message = %q, want bad request", got)
	}
	if got := CodeOf(withData); got != ErrCodeValidate {
		t.Fatalf("extra data error code = %d, want %d", got, ErrCodeValidate)
	}
	if got := ErrorData(withData); got.(map[string]string)["field"] != "tenant_id" {
		t.Fatalf("extra data = %+v", got)
	}
	if WithErrorData(nil, "ignored") != nil {
		t.Fatal("wrapping nil should return nil")
	}
}

func TestRetryableError(t *testing.T) {
	root := NewInternalFailure("temporary failure", nil)
	retryable := WithRetryableError(root)

	if !errors.Is(retryable, root) {
		t.Fatal("retryable error should unwrap its cause")
	}
	if !IsRetryableError(retryable) {
		t.Fatal("retryable error should be detected")
	}
	if got := CodeOf(retryable); got != ErrCodeUnexpected {
		t.Fatalf("retryable error code = %d, want %d", got, ErrCodeUnexpected)
	}
	if WithRetryableError(nil) != nil {
		t.Fatal("wrapping nil should return nil")
	}
	if IsRetryableError(nil) {
		t.Fatal("nil should not be retryable")
	}
}

func TestErrHttpIsError(t *testing.T) {
	err := NewErrHttp(502, []byte("bad gateway"))

	if got := CodeOf(err); got != ErrCodeUnexpected {
		t.Fatalf("http error code = %d, want %d", got, ErrCodeUnexpected)
	}
	if !IsUnexpected(err) {
		t.Fatal("http error should be unexpected")
	}
	if got, want := err.Error(), "http validate failed, status: [502], body : [bad gateway]"; got != want {
		t.Fatalf("http error message = %q, want %q", got, want)
	}
}

func TestCustomCodeError(t *testing.T) {
	err := NewError(29999, "custom", nil)
	if got := CodeOf(err); got != 29999 {
		t.Fatalf("custom code = %d, want 29999", got)
	}
	if !IsErrCode(29999, err) {
		t.Fatal("IsErrCode should use custom error code")
	}
	if !IsExpected(err) {
		t.Fatal("custom expected-range code should be expected")
	}
}

func TestLegacyCompatibilityErrors(t *testing.T) {
	if !errors.Is(NewNotFoundError("missing"), ErrNotFound) {
		t.Fatal("legacy not-found constructor should match ErrNotFound")
	}
	if !IsErrCode(ErrCodeConflict, NewConflictError("conflict")) {
		t.Fatal("legacy conflict constructor should keep conflict code")
	}

	wrapper := &ErrWrapper{Code: 29998, Msg: "wrapped"}
	if got := CodeOf(wrapper); got != 29998 {
		t.Fatalf("legacy wrapper code = %d, want 29998", got)
	}

	callErr := &ErrCall{Method: "GET", Url: "http://example.test", RequestID: "req-1", SrcErr: ErrConflict}
	if !errors.Is(callErr, ErrConflict) {
		t.Fatal("legacy call error should unwrap source error")
	}
}
