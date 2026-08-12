package verify

// IsExactVersionForTest exposes isExactVersion.
func IsExactVersionForTest(v string) bool {
	return isExactVersion(v)
}

// FailClassOfForTest exposes failClassOf.
func FailClassOfForTest(err error) string {
	return failClassOf(err)
}

// ErrStringForTest exposes errString.
func ErrStringForTest(err error) string {
	return errString(err)
}

// NopLoggerVerifyStepForTest calls nopLogger.VerifyStep so the method is covered.
func NopLoggerVerifyStepForTest() {
	nopLogger{}.VerifyStep("step", "ok", "detail")
}

// FilterStepsForTest exposes filterSteps.
func FilterStepsForTest(steps []Step, only []string) []Step {
	return filterSteps(steps, only)
}
