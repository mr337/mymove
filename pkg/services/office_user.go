package services

import (
	"github.com/transcom/mymove/pkg/auth"
	"github.com/transcom/mymove/pkg/models"
)

// OfficeUsersFetcher is the service object for fetching office users
type OfficeUsersFetcher interface {
	FetchOfficeUsers(session *auth.Session) ([]models.OfficeUser, error)
}
