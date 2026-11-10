package infra

import (
	"errors"
	"fmt"
	"log"

	"github.com/GabiHert/phonon/internal/infra/migrate"
)

// migrateNotted copies Notted's data folder over when on is true. Phonon was called Notted. On an
// error it shows alert and returns the error: Phonon must not start, so it never opens an empty
// data folder while Notted's data is still in the old one.
func migrateNotted(p migrate.Plan, on bool, alert func(string)) error {
	if !on {
		return nil
	}
	res, err := p.Run()
	if err != nil {
		if !errors.Is(err, migrate.ErrOldRunning) {
			err = fmt.Errorf("Phonon could not bring over Notted's data, so it did not start: %w", err)
		}
		alert(err.Error())
		return err
	}
	if res.Copied {
		log.Printf("migrate: copied Notted's data folder %s to %s (the original is kept)", p.OldHome, p.NewHome)
	}
	return nil
}
