package base

func DerefInt(i *int) int {
	if i != nil {
		return *i
	}
	return 0
}

func DerefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
