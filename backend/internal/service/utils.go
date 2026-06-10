package service

func derefInt(i *int) int {
	if i != nil {
		return *i
	}
	return 0
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
