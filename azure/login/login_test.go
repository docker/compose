package login

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
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
	mockHelper MockAzureHelper
	azureLogin AzureLoginService
}

func (suite *LoginSuiteTest) BeforeTest(suiteName, testName string) {
	dir, err := ioutil.TempDir("", "test_store")
	Expect(err).To(BeNil())

	suite.dir = dir
	suite.mockHelper = MockAzureHelper{}
	//nolint copylocks
	suite.azureLogin, err = newAzureLoginServiceFromPath(filepath.Join(dir, tokenStoreFilename), suite.mockHelper)
	Expect(err).To(BeNil())
}

func (suite *LoginSuiteTest) AfterTest(suiteName, testName string) {
	err := os.RemoveAll(suite.dir)
	Expect(err).To(BeNil())
}

func (suite *LoginSuiteTest) TestRefreshInValidToken() {
	data := url.Values{
		"grant_type":    []string{"refresh_token"},
		"client_id":     []string{clientID},
		"scope":         []string{scopes},
		"refresh_token": []string{"refreshToken"},
	}
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

func TestLoginSuite(t *testing.T) {
	RegisterTestingT(t)
	suite.Run(t, new(LoginSuiteTest))
}

type MockAzureHelper struct {
	mock.Mock
}

//nolint copylocks
func (s MockAzureHelper) queryToken(data url.Values, tenantID string) (token azureToken, err error) {
	args := s.Called(data, tenantID)
	return args.Get(0).(azureToken), args.Error(1)
}
