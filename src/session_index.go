package tape

func BuildSessionIndex(path string) error {
	return buildSessionIndexWithCodecs(path)
}

func buildSessionIndexWithCodecs(path string, codecs ...EventCodec) error {
	session, err := newSessionFile(path, codecs)
	if err != nil {
		return err
	}
	return session.buildIndex()
}
