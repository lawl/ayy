package parallel

//small helper function to execute multiple functions parallel
//and return immediately if one of them fails
func BailFast(funcs ...func() error) error {
	ch := make(chan error)
	defer close(ch)

	waitFor := 0
	for _, f := range funcs {
		waitFor++
		go func(fn func() error) {
			err := fn()
			ch <- err
		}(f)
	}

	for {
		if waitFor == 0 {
			return nil
		}
		ret := <-ch
		if ret != nil {
			return ret
		}
		waitFor--
	}
}
