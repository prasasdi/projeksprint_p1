package user_service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/levensspel/go-gin-template/cache"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/levensspel/go-gin-template/dto"
	"github.com/levensspel/go-gin-template/entity"
	"github.com/levensspel/go-gin-template/helper"
	"github.com/levensspel/go-gin-template/logger"
	repositories "github.com/levensspel/go-gin-template/repository/employee"
	"github.com/samber/do/v2"
)

const (
	defaultTtl = 5 * time.Minute // Employee data won't more stale than 5 mins
)

type EmployeeService interface {
	Create(ctx context.Context, input dto.EmployeePayload, managerId string) error
	GetAll(ctx context.Context, input dto.GetEmployeesRequest) ([]dto.EmployeePayload, error)
	Update(ctx context.Context, input dto.UpdateEmployeeRequest, identityNumber, managerId string) (dto.EmployeePayload, error)
	Delete(ctx context.Context, identityNumber, managerId string) error
}

type service struct {
	dbPool       *pgxpool.Pool
	employeeRepo repositories.EmployeeRepository
	logger       logger.Logger
}

func NewEmployeeService(
	dbPool *pgxpool.Pool,
	employeeRepo repositories.EmployeeRepository,
	logger logger.Logger,
) EmployeeService {
	return &service{
		dbPool:       dbPool,
		employeeRepo: employeeRepo,
		logger:       logger,
	}
}

func NewEmployeeServiceInject(i do.Injector) (EmployeeService, error) {
	_dbPool := do.MustInvoke[*pgxpool.Pool](i)
	_repo := do.MustInvoke[repositories.EmployeeRepository](i)
	_logger := do.MustInvoke[logger.LogHandler](i)
	return NewEmployeeService(_dbPool, _repo, &_logger), nil
}

func (s *service) Create(ctx context.Context, input dto.EmployeePayload, managerId string) error {
	pool, err := s.dbPool.Begin(ctx)
	if err != nil {
		return helper.ErrInternalServer
	}
	txPool := pool.(*pgxpool.Tx)
	defer helper.RollbackOrCommit(ctx, txPool)

	isOwnedByManager, err := s.employeeRepo.IsDepartmentOwnedByManager(ctx, txPool, input.DepartmentID, managerId)
	if err != nil {
		s.logger.Error(err.Error(), helper.EmployeeServiceGet, err)
		return err
	}

	if !isOwnedByManager {
		return helper.ErrInvalidDepartmentId
	}

	exist, err := s.employeeRepo.IsIdentityNumberExist(ctx, txPool, input.IdentityNumber, managerId)
	if err != nil {
		s.logger.Error(err.Error(), helper.EmployeeServiceGet, err)
		return err
	}
	if exist {
		return helper.ErrConflictIdentityNumber
	}

	err = s.employeeRepo.Insert(ctx, txPool, &input, managerId)
	if err != nil {
		s.logger.Error(err.Error(), helper.EmployeeServiceGet, err)
		if strings.Contains(err.Error(), "23505") {
			return helper.ErrConflict
		}

		return err
	}

	invalidateCache()

	return nil
}

func (s *service) GetAll(ctx context.Context, input dto.GetEmployeesRequest) ([]dto.EmployeePayload, error) {
	cacheKey := s.generateCacheKey(input)

	// Check cache
	cachedEmployees, found := cache.GetAsMapArray(cacheKey)
	if found {
		result := make([]dto.EmployeePayload, len(cachedEmployees))
		for i, employee := range cachedEmployees {
			result[i] = dto.EmployeePayload{
				IdentityNumber:   employee["identityNumber"],
				Name:             employee["name"],
				EmployeeImageUri: employee["employeeImageUri"],
				Gender:           employee["gender"],
				DepartmentID:     employee["departmentId"],
			}
		}
		return result, nil
	}

	employees, err := s.employeeRepo.GetAll(ctx, &input)
	if err != nil {
		s.logger.Error(err.Error(), helper.EmployeeServiceGet, input)
		return []dto.EmployeePayload{}, err
	}

	// Put to cache
	costMultiplier := s.calculateCostMultiplier(input)
	employeesToCache := make([]map[string]string, len(employees))
	for i, employee := range employees {
		employeesToCache[i] = map[string]string{
			"identityNumber":   employee.IdentityNumber,
			"name":             employee.Name,
			"employeeImageUri": employee.EmployeeImageUri,
			"gender":           employee.Gender,
			"departmentId":     employee.DepartmentID,
		}
	}
	cache.SetAsMapArrayWithTtlAndCostMultiplier(cacheKey, employeesToCache, costMultiplier, defaultTtl)

	return employees, nil
}

func (s *service) Update(ctx context.Context, input dto.UpdateEmployeeRequest, identityNumber, managerId string) (dto.EmployeePayload, error) {
	pool, err := s.dbPool.Begin(ctx)
	if err != nil {
		return dto.EmployeePayload{}, helper.ErrInternalServer
	}
	txPool := pool.(*pgxpool.Tx)
	defer helper.RollbackOrCommit(ctx, txPool)

	id, err := s.employeeRepo.GetEmployeeIdIfExist(ctx, txPool, identityNumber, managerId)
	if err != nil {
		s.logger.Error(err.Error(), helper.EmployeeServiceUpdate, err)
		return dto.EmployeePayload{}, err
	}
	if id == "" {
		return dto.EmployeePayload{}, helper.ErrNotFound
	}

	if input.DepartmentID != "" {
		isOwnedByManager, err := s.employeeRepo.IsDepartmentOwnedByManager(ctx, txPool, input.DepartmentID, managerId)
		if err != nil {
			s.logger.Error(err.Error(), helper.EmployeeServiceUpdate, err)
			return dto.EmployeePayload{}, err
		}

		if !isOwnedByManager {
			return dto.EmployeePayload{}, helper.ErrInvalidDepartmentId
		}
	}

	if input.IdentityNumber != "" {
		// Check if the assigned identity number is already used by another employee
		otherEmployeeId, err := s.employeeRepo.GetEmployeeIdIfExist(ctx, txPool, input.IdentityNumber, managerId)
		if err != nil {
			s.logger.Error(err.Error(), helper.EmployeeServiceUpdate, err)
			return dto.EmployeePayload{}, err
		}
		if otherEmployeeId != "" {
			return dto.EmployeePayload{}, helper.ErrConflict
		}
	}

	employee, err := s.employeeRepo.Update(ctx, txPool, entity.Employee{
		Id:               id,
		Name:             input.Name,
		IdentityNumber:   input.IdentityNumber,
		EmployeeImageUri: input.EmployeeImageUri,
		Gender:           input.Gender,
		DepartmentId:     input.DepartmentID,
	})
	if err != nil {
		s.logger.Error(err.Error(), helper.EmployeeServiceUpdate, err)
		if strings.Contains(err.Error(), "23505") {
			return dto.EmployeePayload{}, helper.ErrConflict
		}

		return dto.EmployeePayload{}, err
	}

	invalidateCache()

	return dto.EmployeePayload{
		Name:             employee.Name,
		IdentityNumber:   employee.IdentityNumber,
		EmployeeImageUri: employee.EmployeeImageUri,
		Gender:           employee.Gender,
		DepartmentID:     employee.DepartmentID,
	}, nil
}

func (s *service) Delete(ctx context.Context, identityNumber, managerId string) error {
	pool, err := s.dbPool.Begin(ctx)
	if err != nil {
		return helper.ErrInternalServer
	}
	txPool := pool.(*pgxpool.Tx)
	defer helper.RollbackOrCommit(ctx, txPool)

	id, err := s.employeeRepo.GetEmployeeIdIfExist(ctx, txPool, identityNumber, managerId)
	if err != nil {
		s.logger.Error(err.Error(), helper.EmployeeServiceDelete, err)
		return err
	}
	if id == "" {
		return helper.ErrNotFound
	}

	err = s.employeeRepo.Delete(ctx, txPool, id)
	if err != nil {
		s.logger.Error(err.Error(), helper.EmployeeServiceDelete, err)

		return err
	}

	invalidateCache()

	return nil
}

func (s *service) generateCacheKey(input dto.GetEmployeesRequest) string {
	namespaceVersion := cache.EmployeeNamespaceVersion.Load()

	// Serialize params into a string (e.g., "name=Jono&gender=male")
	var filterParts []string

	filterParts = append(filterParts, fmt.Sprintf("limit=%d", input.Limit))
	filterParts = append(filterParts, fmt.Sprintf("offset=%d", input.Offset))
	filterParts = append(filterParts, fmt.Sprintf("identityNumber=%s", input.IdentityNumber))
	filterParts = append(filterParts, fmt.Sprintf("name=%s", input.Name))
	filterParts = append(filterParts, fmt.Sprintf("gender=%s", input.Gender))
	filterParts = append(filterParts, fmt.Sprintf("departmentId=%s", input.DepartmentID))

	filters := strings.Join(filterParts, "&")
	return fmt.Sprintf(cache.CacheEmployeesWithParams, namespaceVersion, filters)
}

func (s *service) calculateCostMultiplier(input dto.GetEmployeesRequest) int {
	// The more likely it is to be searched, the more beneficial it is to cache
	// By setting higher cost, it will be more likely to be cached and less likely to be evicted

	// No filter
	noFilter := input.IdentityNumber == "" && input.Name == "" && input.Gender == "" && input.DepartmentID == ""
	if noFilter {
		if input.Offset == 1 {
			// First page
			return 4
		} else if input.Offset == 2 {
			// Second page
			return 3
		} else {
			// Subsequent pages
			return 2
		}
	} else {
		return 1
	}
}

func invalidateCache() {
	cache.EmployeeNamespaceVersion.Add(1)
}
