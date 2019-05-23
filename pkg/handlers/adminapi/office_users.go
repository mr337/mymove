package adminapi

import (
	"github.com/go-openapi/runtime/middleware"

	officeuserop "github.com/transcom/mymove/pkg/gen/adminapi/adminoperations/office"
	"github.com/transcom/mymove/pkg/gen/adminmessages"
	"github.com/transcom/mymove/pkg/handlers"
	"github.com/transcom/mymove/pkg/models"
)

// IndexOfficeUsersHandler returns a list of office users via GET /office_users
type IndexOfficeUsersHandler struct {
	handlers.HandlerContext
}

func payloadForOfficeUserModel(ou models.OfficeUser) *adminmessages.OfficeUser {
	officeUserPayload := &adminmessages.OfficeUser{
		ID:        *handlers.FmtUUID(ou.ID),
		Email:     ou.Email,
		FirstName: ou.FirstName,
		LastName:  ou.LastName,
		Telephone: ou.Telephone,
	}

	return officeUserPayload
}

// Handle retrieves a list of office users
func (h IndexOfficeUsersHandler) Handle(params officeuserop.IndexOfficeUsersParams) middleware.Responder {
	db := h.DB()
	var users []models.OfficeUser

	err := db.All(&users)
	if err != nil {
		return officeuserop.NewIndexOfficeUsersBadRequest()
	}

	payload := make(adminmessages.OfficeUsers, len(users))

	for i, u := range users {
		payload[i] = payloadForOfficeUserModel(u)
	}
	return officeuserop.NewIndexOfficeUsersOK()
}
