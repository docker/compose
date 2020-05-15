package login

import (
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"golang.org/x/oauth2"

	. "github.com/onsi/gomega"
)

type LoginSuiteTest struct {
	suite.Suite
	dir        string
	mockHelper *MockAzureHelper
	azureLogin AzureLoginService
}

func (suite *LoginSuiteTest) BeforeTest(suiteName, testName string) {
	dir, err := ioutil.TempDir("", "test_store")
	Expect(err).To(BeNil())

	suite.dir = dir
	suite.mockHelper = &MockAzureHelper{}
	suite.azureLogin, err = newAzureLoginServiceFromPath(filepath.Join(dir, tokenStoreFilename), suite.mockHelper)
	Expect(err).To(BeNil())
}

func (suite *LoginSuiteTest) AfterTest(suiteName, testName string) {
	err := os.RemoveAll(suite.dir)
	Expect(err).To(BeNil())
}

func (suite *LoginSuiteTest) TestRefreshInValidToken() {
	data := refreshTokenData("refreshToken")
	suite.mockHelper.On("queryToken", data, "123456").Return(azureToken{
		RefreshToken: "newRefreshToken",
		AccessToken:  "newAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)

	//nolint copylocks
	azureLogin, err := newAzureLoginServiceFromPath(filepath.Join(suite.dir, tokenStoreFilename), suite.mockHelper)
	Expect(err).To(BeNil())
	suite.azureLogin = azureLogin
	err = suite.azureLogin.tokenStore.writeLoginInfo(TokenInfo{
		TenantID: "123456",
		Token: oauth2.Token{
			AccessToken:  "accessToken",
			RefreshToken: "refreshToken",
			Expiry:       time.Now().Add(-1 * time.Hour),
			TokenType:    "Bearer",
		},
	})
	Expect(err).To(BeNil())

	token, _ := suite.azureLogin.GetValidToken()

	Expect(token.AccessToken).To(Equal("newAccessToken"))
	Expect(token.Expiry).To(BeTemporally(">", time.Now().Add(3500*time.Second)))

	storedToken, _ := suite.azureLogin.tokenStore.readToken()
	Expect(storedToken.Token.AccessToken).To(Equal("newAccessToken"))
	Expect(storedToken.Token.RefreshToken).To(Equal("newRefreshToken"))
	Expect(storedToken.Token.Expiry).To(BeTemporally(">", time.Now().Add(3500*time.Second)))
}

func (suite *LoginSuiteTest) TestDoesNotRefreshValidToken() {
	expiryDate := time.Now().Add(1 * time.Hour)
	err := suite.azureLogin.tokenStore.writeLoginInfo(TokenInfo{
		TenantID: "123456",
		Token: oauth2.Token{
			AccessToken:  "accessToken",
			RefreshToken: "refreshToken",
			Expiry:       expiryDate,
			TokenType:    "Bearer",
		},
	})
	Expect(err).To(BeNil())

	token, _ := suite.azureLogin.GetValidToken()

	Expect(suite.mockHelper.Calls).To(BeEmpty())
	Expect(token.AccessToken).To(Equal("accessToken"))
}

func (suite *LoginSuiteTest) TestInvalidLogin() {
	suite.mockHelper.On("openAzureLoginPage", mock.AnythingOfType("string")).Run(func(args mock.Arguments) {
		redirectURL := args.Get(0).(string)
		err := queryKeyValue(redirectURL, "error", "access denied")
		Expect(err).To(BeNil())
	})

	//nolint copylocks
	azureLogin, err := newAzureLoginServiceFromPath(filepath.Join(suite.dir, tokenStoreFilename), suite.mockHelper)
	Expect(err).To(BeNil())

	err = azureLogin.Login(context.TODO())
	Expect(err).To(MatchError(errors.New("login failed : [access denied]")))
}

func (suite *LoginSuiteTest) TestValidLogin() {
	var redirectURL string
	suite.mockHelper.On("openAzureLoginPage", mock.AnythingOfType("string")).Run(func(args mock.Arguments) {
		redirectURL = args.Get(0).(string)
		err := queryKeyValue(redirectURL, "code", "123456879")
		Expect(err).To(BeNil())
	})

	suite.mockHelper.On("queryToken", mock.MatchedBy(func(data url.Values) bool {
		//Need a matcher here because the value of redirectUrl is not known until executing openAzureLoginPage
		return reflect.DeepEqual(data, url.Values{
			"grant_type":   []string{"authorization_code"},
			"client_id":    []string{clientID},
			"code":         []string{"123456879"},
			"scope":        []string{scopes},
			"redirect_uri": []string{redirectURL},
		})
	}), "organizations").Return(azureToken{
		RefreshToken: "firstRefreshToken",
		AccessToken:  "firstAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)

	authBody := `{"value":[{"id":"/tenants/12345a7c-c56d-43e8-9549-dd230ce8a038","tenantId":"12345a7c-c56d-43e8-9549-dd230ce8a038"}]}`

	suite.mockHelper.On("queryAuthorizationAPI", authorizationURL, "Bearer firstAccessToken").Return([]byte(authBody), 200, nil)
	data := refreshTokenData("firstRefreshToken")
	suite.mockHelper.On("queryToken", data, "12345a7c-c56d-43e8-9549-dd230ce8a038").Return(azureToken{
		RefreshToken: "newRefreshToken",
		AccessToken:  "newAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)
	//nolint copylocks
	azureLogin, err := newAzureLoginServiceFromPath(filepath.Join(suite.dir, tokenStoreFilename), suite.mockHelper)
	Expect(err).To(BeNil())

	err = azureLogin.Login(context.TODO())
	Expect(err).To(BeNil())

	loginToken, err := suite.azureLogin.tokenStore.readToken()
	Expect(err).To(BeNil())
	Expect(loginToken.Token.AccessToken).To(Equal("newAccessToken"))
	Expect(loginToken.Token.RefreshToken).To(Equal("newRefreshToken"))
	Expect(loginToken.Token.Expiry).To(BeTemporally(">", time.Now().Add(3500*time.Second)))
	Expect(loginToken.TenantID).To(Equal("12345a7c-c56d-43e8-9549-dd230ce8a038"))
	Expect(loginToken.Token.Type()).To(Equal("Bearer"))
}

func (suite *LoginSuiteTest) TestLoginAuthorizationFailed() {
	var redirectURL string
	suite.mockHelper.On("openAzureLoginPage", mock.AnythingOfType("string")).Run(func(args mock.Arguments) {
		redirectURL = args.Get(0).(string)
		err := queryKeyValue(redirectURL, "code", "123456879")
		Expect(err).To(BeNil())
	})

	suite.mockHelper.On("queryToken", mock.MatchedBy(func(data url.Values) bool {
		//Need a matcher here because the value of redirectUrl is not known until executing openAzureLoginPage
		return reflect.DeepEqual(data, url.Values{
			"grant_type":   []string{"authorization_code"},
			"client_id":    []string{clientID},
			"code":         []string{"123456879"},
			"scope":        []string{scopes},
			"redirect_uri": []string{redirectURL},
		})
	}), "organizations").Return(azureToken{
		RefreshToken: "firstRefreshToken",
		AccessToken:  "firstAccessToken",
		ExpiresIn:    3600,
		Foci:         "1",
	}, nil)

	authBody := `[access denied]`

	suite.mockHelper.On("queryAuthorizationAPI", authorizationURL, "Bearer firstAccessToken").Return([]byte(authBody), 400, nil)

	azureLogin, err := newAzureLoginServiceFromPath(filepath.Join(suite.dir, tokenStoreFilename), suite.mockHelper)
	Expect(err).To(BeNil())

	err = azureLogin.Login(context.TODO())
	Expect(err).To(MatchError(errors.New("login failed : [access denied]")))
}

func refreshTokenData(refreshToken string) url.Values {
	return url.Values{
		"grant_type":    []string{"refresh_token"},
		"client_id":     []string{clientID},
		"scope":         []string{scopes},
		"refresh_token": []string{refreshToken},
	}
}

func queryKeyValue(redirectURL string, key string, value string) error {
	req, err := http.NewRequest("GET", redirectURL, nil)
	Expect(err).To(BeNil())
	q := req.URL.Query()
	q.Add(key, value)
	req.URL.RawQuery = q.Encode()
	client := &http.Client{}
	_, err = client.Do(req)
	return err
}

func TestLoginSuite(t *testing.T) {
	RegisterTestingT(t)
	suite.Run(t, new(LoginSuiteTest))
}

type MockAzureHelper struct {
	mock.Mock
}

func (s *MockAzureHelper) queryToken(data url.Values, tenantID string) (token azureToken, err error) {
	args := s.Called(data, tenantID)
	return args.Get(0).(azureToken), args.Error(1)
}

func (s *MockAzureHelper) queryAuthorizationAPI(authorizationURL string, authorizationHeader string) ([]byte, int, error) {
	args := s.Called(authorizationURL, authorizationHeader)
	return args.Get(0).([]byte), args.Int(1), args.Error(2)
}

func (s *MockAzureHelper) openAzureLoginPage(redirectURL string) {
	s.Called(redirectURL)
}
