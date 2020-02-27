package wineventlog

import "testing"

func TestEventIterator(t *testing.T) {
	query := openLog(t, security4752File)
	defer query.Close()

	itr := NewEventIterator(query, 512)

	for itr.Next() {
		handleEvent(itr.Handle())
	}
	if err := itr.Err(); err != nil {
		t.Fatal(err)
	}
}

func handleEvent(evt EvtHandle) {
	defer evt.Close()
}
