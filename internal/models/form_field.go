// internal/models/form_field.go
package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type FormFieldType string

const (
	FieldText     FormFieldType = "text"
	FieldEmail    FormFieldType = "email"
	FieldTel      FormFieldType = "tel"
	FieldTextarea FormFieldType = "textarea"
	FieldSelect   FormFieldType = "select"
	FieldCheckbox FormFieldType = "checkbox"
	FieldRadio    FormFieldType = "radio"
	FieldNumber   FormFieldType = "number"
	FieldDate     FormFieldType = "date"
)

type FormField struct {
	ID string `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`

	FormID string `gorm:"type:uuid;not null;index" json:"formId"`

	// "key" is used in submission values map
	Key      string       `gorm:"size:100;not null" json:"key"`
	Label    string       `gorm:"size:255;not null" json:"label"`
	Type     FormFieldType `gorm:"size:30;not null" json:"type"`
	Required bool         `gorm:"not null;default:false" json:"required"`

	// JSON array of options: [{label,value}]
	Options datatypes.JSON `gorm:"type:jsonb" json:"options,omitempty"`

	Order int `gorm:"not null;default:0" json:"order"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (FormField) TableName() string {
	return "form_fields"
}
