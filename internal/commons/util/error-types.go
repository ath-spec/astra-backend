package util

import (
	"errors"
	"net/http"
)

func ErrRequiredInputMissing(field string) error {
	return errors.New("required input missing: " + field)
}

func ErrUnchangeable(field string) error {
	return errors.New("field unchangeable after creation: " + field)
}

func ErrHeaderMissing(field string) error {
	return errors.New("header missing: " + field)
}

var (
	ErrExpiredToken                          = errors.New("expired token")
	ErrInvalidToken                          = errors.New("invalid token")
	ErrInvalidOtp                            = errors.New("invalid otp")
	ErrInternal                              = errors.New("internal error")
	ErrNotFound                              = errors.New("not found")
	ErrTokenMissing                          = errors.New("token missing")
	ErrContextMissing                        = errors.New("context missing")
	ErrUnauthorized                          = errors.New("unauthorized")
	ErrInvalidRole                           = errors.New("invalid role")
	ErrEnvironment                           = errors.New("environment error")
	ErrDatabase                              = errors.New("database error")
	ErrEmptyResult                           = errors.New("empty result")
	ErrTooManyRequests                       = errors.New("too many requests")
	ErrEntryExists                           = errors.New("entry already exists")
	ErrInvalidUser                           = errors.New("invalid user")
	ErrSessionExpired                        = errors.New("session expired (something fishy may be happening)")
	ErrSmsUnableToSend                       = errors.New("unable to send sms")
	ErrExpiredOtp                            = errors.New("expired otp")
	ErrVinAlreadyPaired                      = errors.New("vin already paired")
	ErrVinNotPaired                          = errors.New("vin not paired to your profile")
	ErrInvalidPin                            = errors.New("invalid pin")
	ErrFileNotFound                          = errors.New("file not found")
	ErrEmailAlreadyVerified                  = errors.New("email already verified")
	ErrEmailNotVerified                      = errors.New("email not verified")
	ErrUrlParamsMissing                      = errors.New("url params missing")
	ErrModelDoesNotExist                     = errors.New("model does not exist")
	ErrInvalidFileFormat                     = errors.New("invalid file format")
	ErrSamePin                               = errors.New("same pin")
	ErrTooManyEntries                        = errors.New("too many entries")
	ErrInvalidObjectId                       = errors.New("invalid object id")
	ErrLocationNotFound                      = errors.New("location not found")
	ErrCouldNotSaveNotification              = errors.New("could not save notification")
	ErrCouldNotReestablishDatabaseConnection = errors.New("could not reestablish database connection")
	ErrEventCreationLimitExceeded            = errors.New("event creation limit exceeded")
	ErrAlreadyRegistered                     = errors.New("already registered")
	ErrNotValidRequest                       = errors.New("not valid request")
	ErrMultipartFileMissing                  = errors.New("multipart file missing")
	ErrNotInLocation                         = errors.New("Not in location")
	ErrInvalidUuid                           = errors.New("invalid uuid")
	ErrGrpcConnFailed                        = errors.New("grpc connection failed")
	ErrGrpcQueryError                        = errors.New("grpc could not execute query")
	ErrNoGpsMapped                           = errors.New("no gps mapped to this profile")
	ErrGrpcCouldNotExecuteQuery              = errors.New("grpc could not execute query")
	ErrUserAlreadyExsists                    = errors.New("user already exsists")
	ErrWrongPassword                         = errors.New("wrong password")
	ErrNotEnoughCoins                        = errors.New("not enough coins for voucher")
	ErrInputInErrData                        = errors.New("Errdata should contain either 0 or 1")
	ErrFramenumber                           = errors.New("invalid frame number input")
	ErrFrameExists                           = errors.New("frame number already exists")
	ErrInvalidQueryParams                    = errors.New("invalid query parameters")
	ErrAccessTokenGenError                   = errors.New("error in generating access token")
	ErrRefreshTokenGenError                  = errors.New("error in generating refresh token")
	ErrForbidden                             = errors.New("You can only add upto 5 favourite locations")
	ErrTripIdMissing                         = errors.New("No tripId passed")
	ErrMultipleLogin                         = errors.New("Multiple Login")
	ErrMinLatLng                             = errors.New("at least two positions are required to create a map")
	ErrMapUrl                                = errors.New("error fetching the image")
	ErrInvalidPAN                            = errors.New("invalid PAN number format")
	ErrInvalidPhoneNumber                    = errors.New("invalid phone number format")
	ErrInvalidIncomeBracket                  = errors.New("invalid income bracket")
	ErrInvalidFinancialGoals                 = errors.New("invalid financial goals format")
	ErrSuspiciousActivity                    = errors.New("suspicious activity detected")
	ErrAccountLocked                         = errors.New("account temporarily locked")
	// Bank Parser specific errors
	ErrBankStatementNotFound           = errors.New("bank statement not found")
	ErrBankStatementAccessDenied       = errors.New("access denied to bank statement")
	ErrInvalidBankStatementId          = errors.New("invalid bank statement ID")
	ErrBankStatementProcessingFailed   = errors.New("bank statement processing failed")
	ErrBankStatementAlreadyExists      = errors.New("bank statement already exists")
	ErrInvalidBankStatementFormat      = errors.New("invalid bank statement format")
	ErrBankStatementUploadFailed       = errors.New("bank statement upload failed")
	ErrBankStatementParseFailed        = errors.New("bank statement parsing failed")
	ErrBankStatementTooLarge           = errors.New("bank statement file too large")
	ErrBankStatementUnsupportedFormat  = errors.New("unsupported bank statement format")
	ErrBankStatementCorrupted          = errors.New("bank statement file is corrupted")
	ErrBankStatementEmpty              = errors.New("bank statement is empty")
	ErrBankStatementExpired            = errors.New("bank statement has expired")
	ErrBankStatementIncomplete         = errors.New("bank statement is incomplete")
	ErrBankStatementValidationFailed   = errors.New("bank statement validation failed")
	ErrBankStatementStorageFailed      = errors.New("failed to store bank statement")
	ErrBankStatementRetrievalFailed    = errors.New("failed to retrieve bank statement")
	ErrBankStatementDeletionFailed     = errors.New("failed to delete bank statement")
	ErrBankStatementUpdateFailed       = errors.New("failed to update bank statement")
	ErrBankStatementQuotaExceeded      = errors.New("bank statement quota exceeded")
	ErrBankStatementServiceUnavailable = errors.New("bank statement service unavailable")
	// SMS Parser specific errors
	ErrSmsTransactionNotFound           = errors.New("SMS transaction not found")
	ErrSmsTransactionAccessDenied       = errors.New("access denied to SMS transaction")
	ErrInvalidSmsTransactionId          = errors.New("invalid SMS transaction ID")
	ErrSmsTransactionProcessingFailed   = errors.New("SMS transaction processing failed")
	ErrSmsTransactionAlreadyExists      = errors.New("SMS transaction already exists")
	ErrInvalidSmsTransactionFormat      = errors.New("invalid SMS transaction format")
	ErrSmsTransactionUploadFailed       = errors.New("SMS transaction upload failed")
	ErrSmsTransactionParseFailed        = errors.New("SMS transaction parsing failed")
	ErrSmsTransactionTooLarge           = errors.New("SMS transaction too large")
	ErrSmsTransactionUnsupportedFormat  = errors.New("unsupported SMS transaction format")
	ErrSmsTransactionCorrupted          = errors.New("SMS transaction is corrupted")
	ErrSmsTransactionEmpty              = errors.New("SMS transaction is empty")
	ErrSmsTransactionExpired            = errors.New("SMS transaction has expired")
	ErrSmsTransactionIncomplete         = errors.New("SMS transaction is incomplete")
	ErrSmsTransactionValidationFailed   = errors.New("SMS transaction validation failed")
	ErrSmsTransactionStorageFailed      = errors.New("failed to store SMS transaction")
	ErrSmsTransactionRetrievalFailed    = errors.New("failed to retrieve SMS transaction")
	ErrSmsTransactionDeletionFailed     = errors.New("failed to delete SMS transaction")
	ErrSmsTransactionUpdateFailed       = errors.New("failed to update SMS transaction")
	ErrSmsTransactionQuotaExceeded      = errors.New("SMS transaction quota exceeded")
	ErrSmsTransactionServiceUnavailable = errors.New("SMS transaction service unavailable")
	ErrSmsParsingRuleNotFound           = errors.New("SMS parsing rule not found")
	ErrInvalidSmsParsingRule            = errors.New("invalid SMS parsing rule")
	ErrSmsParsingRuleCreationFailed     = errors.New("failed to create SMS parsing rule")
	ErrSmsParsingRuleUpdateFailed       = errors.New("failed to update SMS parsing rule")
	ErrSmsParsingRuleDeletionFailed     = errors.New("failed to delete SMS parsing rule")
	ErrMerchantMetadataNotFound         = errors.New("merchant metadata not found")
	ErrInvalidMerchantMetadata          = errors.New("invalid merchant metadata")
	ErrMerchantMetadataCreationFailed   = errors.New("failed to create merchant metadata")
	ErrMerchantMetadataUpdateFailed     = errors.New("failed to update merchant metadata")
	ErrMerchantMetadataDeletionFailed   = errors.New("failed to delete merchant metadata")
	ErrSmsDeduplicationFailed           = errors.New("SMS deduplication failed")
	ErrSmsNormalizationFailed           = errors.New("SMS normalization failed")
	ErrSmsEnrichmentFailed              = errors.New("SMS enrichment failed")
	ErrSmsConsentNotGiven               = errors.New("SMS consent not given")
	ErrSmsConsentExpired                = errors.New("SMS consent expired")
	ErrSmsEncryptionFailed              = errors.New("SMS encryption failed")
	ErrSmsDecryptionFailed              = errors.New("SMS decryption failed")
	// Neo4j specific errors
	ErrNeo4jConnectionFailed = errors.New("failed to connect to neo4j")
	ErrNeo4jExecutionFailed  = errors.New("failed to execute neo4j query")
	ErrNeo4jQueryFailed      = errors.New("neo4j query returned error")
	//otp error
	ErrOtpLimitExceeded      = errors.New("otp limit exceeded try after 2 hours")
)

var CustomErrorType = map[error]int{
	ErrExpiredToken:                          http.StatusUnauthorized,
	ErrInvalidToken:                          http.StatusUnauthorized,
	ErrInvalidOtp:                            419, // defining 419 for when a users token doesn't exist in cache
	ErrInternal:                              http.StatusInternalServerError,
	ErrNotFound:                              http.StatusNotFound,
	ErrTokenMissing:                          http.StatusUnauthorized,
	ErrContextMissing:                        http.StatusInternalServerError,
	ErrUnauthorized:                          http.StatusForbidden,
	ErrInvalidRole:                           http.StatusBadRequest,
	ErrEnvironment:                           http.StatusInternalServerError,
	ErrDatabase:                              http.StatusInternalServerError,
	ErrEmptyResult:                           http.StatusBadRequest,
	ErrTooManyRequests:                       http.StatusTooManyRequests,
	ErrEntryExists:                           http.StatusBadRequest,
	ErrInvalidUser:                           http.StatusBadRequest,
	ErrSessionExpired:                        http.StatusUnauthorized, // maps to 401
	ErrSmsUnableToSend:                       430, // defining 430 for when an SMS is unable to send
	ErrExpiredOtp:                            429,
	ErrVinAlreadyPaired:                      427, // defining 427 for when a vin is already paired
	ErrVinNotPaired:                          http.StatusBadRequest,
	ErrFileNotFound:                          http.StatusInternalServerError,
	ErrEmailAlreadyVerified:                  http.StatusBadRequest,
	ErrInvalidPin:                            http.StatusBadRequest,
	ErrEmailNotVerified:                      http.StatusBadRequest,
	ErrModelDoesNotExist:                     http.StatusBadRequest,
	ErrInvalidFileFormat:                     http.StatusBadRequest,
	ErrUrlParamsMissing:                      http.StatusBadRequest,
	ErrSamePin:                               http.StatusBadRequest,
	ErrTooManyEntries:                        http.StatusBadRequest,
	ErrInvalidObjectId:                       http.StatusBadRequest,
	ErrLocationNotFound:                      http.StatusBadRequest,
	ErrCouldNotSaveNotification:              http.StatusInternalServerError,
	ErrCouldNotReestablishDatabaseConnection: http.StatusInternalServerError,
	ErrInputInErrData:                        http.StatusNotAcceptable,
	ErrFramenumber:                           422,
	ErrFrameExists:                           427,
	ErrInvalidQueryParams:                    http.StatusBadRequest,
	ErrAccessTokenGenError:                   http.StatusInternalServerError,
	ErrRefreshTokenGenError:                  http.StatusInternalServerError,
	ErrForbidden:                             http.StatusForbidden,
	ErrTripIdMissing:                         http.StatusBadRequest,
	ErrMultipleLogin:                         http.StatusUnauthorized,
	ErrMinLatLng:                             http.StatusBadRequest,
	ErrMapUrl:                                403,
	ErrInvalidPAN:                            http.StatusBadRequest,
	ErrInvalidPhoneNumber:                    http.StatusBadRequest,
	ErrInvalidIncomeBracket:                  http.StatusBadRequest,
	ErrInvalidFinancialGoals:                 http.StatusBadRequest,
	ErrSuspiciousActivity:                    http.StatusForbidden,
	ErrAccountLocked:                         http.StatusLocked,
	// Bank Parser specific error codes
	ErrBankStatementNotFound:           http.StatusNotFound,
	ErrBankStatementAccessDenied:       http.StatusForbidden,
	ErrInvalidBankStatementId:          http.StatusBadRequest,
	ErrBankStatementProcessingFailed:   http.StatusInternalServerError,
	ErrBankStatementAlreadyExists:      http.StatusConflict,
	ErrInvalidBankStatementFormat:      http.StatusBadRequest,
	ErrBankStatementUploadFailed:       http.StatusInternalServerError,
	ErrBankStatementParseFailed:        http.StatusInternalServerError,
	ErrBankStatementTooLarge:           http.StatusRequestEntityTooLarge,
	ErrBankStatementUnsupportedFormat:  http.StatusUnsupportedMediaType,
	ErrBankStatementCorrupted:          http.StatusBadRequest,
	ErrBankStatementEmpty:              http.StatusBadRequest,
	ErrBankStatementExpired:            http.StatusGone,
	ErrBankStatementIncomplete:         http.StatusBadRequest,
	ErrBankStatementValidationFailed:   http.StatusBadRequest,
	ErrBankStatementStorageFailed:      http.StatusInternalServerError,
	ErrBankStatementRetrievalFailed:    http.StatusInternalServerError,
	ErrBankStatementDeletionFailed:     http.StatusInternalServerError,
	ErrBankStatementUpdateFailed:       http.StatusInternalServerError,
	ErrBankStatementQuotaExceeded:      http.StatusTooManyRequests,
	ErrBankStatementServiceUnavailable: http.StatusServiceUnavailable,
	// SMS Parser specific error codes
	ErrSmsTransactionNotFound:           http.StatusNotFound,
	ErrSmsTransactionAccessDenied:       http.StatusForbidden,
	ErrInvalidSmsTransactionId:          http.StatusBadRequest,
	ErrSmsTransactionProcessingFailed:   http.StatusInternalServerError,
	ErrSmsTransactionAlreadyExists:      http.StatusConflict,
	ErrInvalidSmsTransactionFormat:      http.StatusBadRequest,
	ErrSmsTransactionUploadFailed:       http.StatusInternalServerError,
	ErrSmsTransactionParseFailed:        http.StatusInternalServerError,
	ErrSmsTransactionTooLarge:           http.StatusRequestEntityTooLarge,
	ErrSmsTransactionUnsupportedFormat:  http.StatusUnsupportedMediaType,
	ErrSmsTransactionCorrupted:          http.StatusBadRequest,
	ErrSmsTransactionEmpty:              http.StatusBadRequest,
	ErrSmsTransactionExpired:            http.StatusGone,
	ErrSmsTransactionIncomplete:         http.StatusBadRequest,
	ErrSmsTransactionValidationFailed:   http.StatusBadRequest,
	ErrSmsTransactionStorageFailed:      http.StatusInternalServerError,
	ErrSmsTransactionRetrievalFailed:    http.StatusInternalServerError,
	ErrSmsTransactionDeletionFailed:     http.StatusInternalServerError,
	ErrSmsTransactionUpdateFailed:       http.StatusInternalServerError,
	ErrSmsTransactionQuotaExceeded:      http.StatusTooManyRequests,
	ErrSmsTransactionServiceUnavailable: http.StatusServiceUnavailable,
	ErrSmsParsingRuleNotFound:           http.StatusNotFound,
	ErrInvalidSmsParsingRule:            http.StatusBadRequest,
	ErrSmsParsingRuleCreationFailed:     http.StatusInternalServerError,
	ErrSmsParsingRuleUpdateFailed:       http.StatusInternalServerError,
	ErrSmsParsingRuleDeletionFailed:     http.StatusInternalServerError,
	ErrMerchantMetadataNotFound:         http.StatusNotFound,
	ErrInvalidMerchantMetadata:          http.StatusBadRequest,
	ErrMerchantMetadataCreationFailed:   http.StatusInternalServerError,
	ErrMerchantMetadataUpdateFailed:     http.StatusInternalServerError,
	ErrMerchantMetadataDeletionFailed:   http.StatusInternalServerError,
	ErrSmsDeduplicationFailed:           http.StatusInternalServerError,
	ErrSmsNormalizationFailed:           http.StatusInternalServerError,
	ErrSmsEnrichmentFailed:              http.StatusInternalServerError,
	ErrSmsConsentNotGiven:               http.StatusForbidden,
	ErrSmsConsentExpired:                http.StatusGone,
	ErrSmsEncryptionFailed:              http.StatusInternalServerError,
	ErrSmsDecryptionFailed:              http.StatusInternalServerError,
	// Neo4j specific error codes
	ErrNeo4jConnectionFailed: http.StatusServiceUnavailable,
	ErrNeo4jExecutionFailed:  http.StatusInternalServerError,
	ErrNeo4jQueryFailed:      http.StatusBadRequest,
	ErrOtpLimitExceeded:      http.StatusTooManyRequests,
}
