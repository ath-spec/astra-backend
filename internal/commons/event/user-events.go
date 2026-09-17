package event

import (
	"encoding/json"

	"github.com/google/uuid"
)

type UserBike struct {
	Zyid         uuid.UUID `json:"emuser_id"`
	BikeVin      string    `json:"bike_vin"`
	BikeModel    string    `json:"bike_model"`
	MqttPassword string    `json:"mqtt_password"`
}

func (p *producer) ProduceUserBike(id uuid.UUID, vin, model, pass string) error {
	j, _ := json.Marshal(UserBike{
		Zyid:         id,
		BikeVin:      vin,
		BikeModel:    model,
		MqttPassword: pass,
	})
	return p.produce(j, USERBIKE)
}

type UserProfileAdded struct {
	Zyid                    uuid.UUID `json:"emuser_id"`
	Username                string    `json:"user_name"`
	Phone                   string    `json:"phone"`
	UserRole                string    `json:"role"`
	ProfilePictureExtension string    `json:"profile_picture_extension"`
	ProfilePicture          string    `json:"profile_picture"`
}

func (p *producer) ProduceProfileAdded(id uuid.UUID, username string, phone string, userrole string, profilepictureextension string, profilepicture string) error {
	j, _ := json.Marshal(UserProfileAdded{
		Zyid:                    id,
		Username:                username,
		Phone:                   phone,
		UserRole:                userrole,
		ProfilePicture:          profilepicture,
		ProfilePictureExtension: profilepictureextension,
	})
	return p.produce(j, USER_PROFILE_ADDED)
}

type UserProfileDeleted struct {
	Zyid  uuid.UUID `json:"emuser_id"`
	Vin   string    `json:"vin"`
	Phone string    `json:"phone"`
}

func (p *producer) ProduceProfileDeleted(id uuid.UUID, vin string, phone string) error {
	j, _ := json.Marshal(UserProfileDeleted{
		Zyid:  id,
		Vin:   vin,
		Phone: phone,
	})
	return p.produce(j, USER_PROFILE_DELETED)
}

type User struct {
	Zyid     uuid.UUID `json:"emuser_id"`
	Phone    string    `json:"phone"`
	UserRole string    `json:"role"`
}

func (p *producer) ProduceUser(phone string, id uuid.UUID) error {
	j, _ := json.Marshal(User{
		Phone: phone,
		Zyid:  id,
	})
	return p.produce(j, USER)
}

type Role struct {
	Zyid     uuid.UUID `json:"emuser_id"`
	UserRole string    `json:"role"`
}

func (p *producer) ProduceRole(id uuid.UUID, userrole string) error {
	j, _ := json.Marshal(Role{
		UserRole: userrole,
		Zyid:     id,
	})
	return p.produce(j, ROLE)
}

type Email struct {
	Zyid  uuid.UUID `json:"emuser_id"`
	Email string    `json:"email"`
}

func (p *producer) ProduceEmail(email string, id uuid.UUID) error {

	j, _ := json.Marshal(Email{
		Email: email,
		Zyid:  id,
	})
	return p.produce(j, EMAIL)
}
