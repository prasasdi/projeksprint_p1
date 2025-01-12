package validation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/levensspel/go-gin-template/dto"
)

func ValidateEmployeeCreate(input *dto.EmployeePayload) error {
	if input.IdentityNumber == "" {
		return errors.New("Invalid identity number param")
	}
	if input.Name == "" {
		return errors.New("Invalid name param")
	}
	if input.EmployeeImageUri == "" {
		return errors.New("Invalid image URI param")
	}
	if input.DepartmentID == "" {
		return errors.New("Invalid department id param")
	}

	isImageUriValid := isImageURIExtensionValid(input.EmployeeImageUri)
	if !isImageUriValid {
		return errors.New("Invalid image URI param")
	}

	gender := strings.ToLower(input.Gender)
	// fmt.Println("gender:", strings.ToLower(gender))
	switch gender {
	case "":
		input.Gender = ""
	case dto.GenderMale:
		input.Gender = dto.GenderMale
		break
	case dto.GenderFemale:
		input.Gender = dto.GenderFemale
		break
	default:
		return errors.New("Invalid gender param")
	}

	err := validate.Struct(input)
	if err != nil {
		validationErrors := err.(validator.ValidationErrors)
		for _, fieldError := range validationErrors {
			return fieldError
		}
	}
	return nil
}

func ValidateEmployeeGet(input *dto.GetEmployeesRequest) error {
	gender := strings.ToLower(input.Gender)
	// fmt.Println("gender:", strings.ToLower(gender))
	switch gender {
	case "":
		input.Gender = ""
	case dto.GenderMale:
		input.Gender = dto.GenderMale
		break
	case dto.GenderFemale:
		input.Gender = dto.GenderFemale
		break
	default:
		return errors.New("Invalid gender param")
	}

	err := validate.Struct(input)
	if err != nil {
		validationErrors := err.(validator.ValidationErrors)
		for _, fieldError := range validationErrors {
			return fieldError
		}
	}
	return nil
}

// Validate each JSON properties whether sent or not, then copy all its values to the target variable referenced
func ValidateEmployeeUpdate(input map[string]string, target *dto.UpdateEmployeeRequest) error {
	identityNumber, isIdentityNumberSent := input["identityNumber"]
	if isIdentityNumberSent && identityNumber == "" {
		return errors.New("Invalid identity number param")
	}

	name, isNameSent := input["name"]
	if isNameSent && name == "" {
		return errors.New("Invalid name param")
	}

	employeeImageUri, isEmployeeImageUriSent := input["employeeImageUri"]
	if isEmployeeImageUriSent && employeeImageUri == "" {
		return errors.New("Invalid image URI param")
	}

	gender, isGenderSent := input["gender"]
	if isGenderSent && gender == "" {
		return errors.New("Invalid gender param")
	}

	departmentId, isDepartmentId := input["departmentId"]
	if isDepartmentId && departmentId == "" {
		return errors.New("Invalid department id param")
	}

	isImageUriValid := isImageURIExtensionValid(employeeImageUri)
	if !isImageUriValid {
		return errors.New("Invalid image URI param")
	}

	if isGenderSent {
		switch gender {
		case "":
			target.Gender = ""
		case dto.GenderMale:
			target.Gender = dto.GenderMale
			break
		case dto.GenderFemale:
			target.Gender = dto.GenderFemale
			break
		default:
			return errors.New("Invalid gender param")
		}
	}

	target.IdentityNumber = identityNumber
	target.Name = name
	target.EmployeeImageUri = employeeImageUri
	target.DepartmentID = departmentId

	err := validate.Struct(target)
	if err != nil {
		validationErrors := err.(validator.ValidationErrors)
		for _, fieldError := range validationErrors {

			return fieldError
		}
	}
	return nil
}

func isImageURIExtensionValid(uri string) bool {
	extension := uri[len(uri)-5:]
	if extension != ".jpeg" {
		extension = extension[1:]
		if extension != ".jpg" && extension != ".png" {
			fmt.Println("false", extension)
			return false
		}
	}

	return true
}
