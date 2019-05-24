package officeuser

import (
	"fmt"

	"github.com/gobuffalo/pop"
	"github.com/pkg/errors"

	"github.com/transcom/mymove/pkg/auth"
	"github.com/transcom/mymove/pkg/models"
	"github.com/transcom/mymove/pkg/services"
)

type fetchOfficeUsers struct {
	db *pop.Connection
}

func (f *fetchOfficeUsers) FetchOfficeUsers(session *auth.Session) ([]models.OfficeUser, error) {
	var users []models.OfficeUser

	err := f.db.All(&users)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("%s", err))
	}

	return users, nil
}

// NewOfficeUsersFetcher is the public constructor for the fetchOfficeUsers type
func NewOfficeUsersFetcher(db *pop.Connection) services.OfficeUsersFetcher {
	return &fetchOfficeUsers{db}
}
