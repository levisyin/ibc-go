package keeper_test

import (
	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"

	"github.com/cosmos/ibc-go/v8/modules/apps/29-fee/keeper"
	"github.com/cosmos/ibc-go/v8/modules/apps/29-fee/types"
	channeltypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
)

func (suite *KeeperTestSuite) TestLegacyTotal() {
	fee := types.NewFee(defaultRecvFee, defaultAckFee, defaultTimeoutFee)
	expLegacyTotal := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(600)))

	suite.Require().Equal(expLegacyTotal, keeper.LegacyTotal(fee))
}

func (suite *KeeperTestSuite) TestMigrate1to2() {
	var (
		packetID         channeltypes.PacketId
		moduleAcc        sdk.AccAddress
		refundAcc        sdk.AccAddress
		initRefundAccBal sdk.Coins
		initModuleAccBal sdk.Coins
		packetFees       []types.PacketFee
	)

	testCases := []struct {
		name     string
		malleate func()
		assert   func(error)
	}{
		{
			"success: no fees in escrow",
			func() {},
			func(err error) {
				suite.Require().NoError(err)
				suite.Require().Empty(suite.chainA.GetSimApp().IBCFeeKeeper.GetAllIdentifiedPacketFees(suite.chainA.GetContext()))

				// refund account balance should not change
				refundAccBal := suite.chainA.GetSimApp().BankKeeper.GetBalance(suite.chainA.GetContext(), refundAcc, sdk.DefaultBondDenom)
				suite.Require().Equal(initRefundAccBal[0], refundAccBal)

				// module account balance should not change
				moduleAccBal := suite.chainA.GetSimApp().BankKeeper.GetBalance(suite.chainA.GetContext(), moduleAcc, sdk.DefaultBondDenom)
				suite.Require().True(moduleAccBal.IsZero())
			},
		},
		{
			"success: one fee in escrow",
			func() {
				fee := types.NewFee(defaultRecvFee, defaultAckFee, defaultTimeoutFee)
				packetFee := types.NewPacketFee(fee, refundAcc.String(), []string(nil))
				packetFees = []types.PacketFee{packetFee}
			},
			func(err error) {
				suite.Require().NoError(err)

				// ensure that the packet fees are unmodified
				expPacketFees := []types.IdentifiedPacketFees{
					types.NewIdentifiedPacketFees(packetID, packetFees),
				}
				suite.Require().Equal(expPacketFees, suite.chainA.GetSimApp().IBCFeeKeeper.GetAllIdentifiedPacketFees(suite.chainA.GetContext()))

				unusedFee := sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(300))
				// refund account balance should increase
				refundAccBal := suite.chainA.GetSimApp().BankKeeper.GetBalance(suite.chainA.GetContext(), refundAcc, sdk.DefaultBondDenom)
				suite.Require().Equal(initRefundAccBal.Add(unusedFee)[0], refundAccBal)

				// module account balance should decrease
				moduleAccBal := suite.chainA.GetSimApp().BankKeeper.GetBalance(suite.chainA.GetContext(), moduleAcc, sdk.DefaultBondDenom)
				suite.Require().Equal(initModuleAccBal.Sub(unusedFee)[0], moduleAccBal)
			},
		},
		{
			"success: many fees with multiple denoms in escrow",
			func() {
				fee1 := types.NewFee(defaultRecvFee, defaultAckFee, defaultTimeoutFee)
				packetFee1 := types.NewPacketFee(fee1, refundAcc.String(), []string(nil))

				// mint some tokens to the refund account
				denom2 := "denom"
				err := suite.chainA.GetSimApp().MintKeeper.MintCoins(suite.chainA.GetContext(), sdk.NewCoins(sdk.NewCoin(denom2, sdkmath.NewInt(1000))))
				suite.Require().NoError(err)
				err = suite.chainA.GetSimApp().BankKeeper.SendCoinsFromModuleToAccount(suite.chainA.GetContext(), minttypes.ModuleName, refundAcc, sdk.NewCoins(sdk.NewCoin(denom2, sdkmath.NewInt(1000))))
				suite.Require().NoError(err)

				defaultFee2 := sdk.NewCoins(sdk.NewCoin(denom2, sdkmath.NewInt(100)))
				fee2 := types.NewFee(defaultFee2, defaultFee2, defaultFee2)
				packetFee2 := types.NewPacketFee(fee2, refundAcc.String(), []string(nil))

				packetFees = []types.PacketFee{packetFee1, packetFee2, packetFee1}
			},
			func(err error) {
				denom2 := "denom"

				suite.Require().NoError(err)

				// ensure that the packet fees are unmodified
				expPacketFees := []types.IdentifiedPacketFees{
					types.NewIdentifiedPacketFees(packetID, packetFees),
				}
				suite.Require().Equal(expPacketFees, suite.chainA.GetSimApp().IBCFeeKeeper.GetAllIdentifiedPacketFees(suite.chainA.GetContext()))

				unusedFee := sdk.NewCoins(
					sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(600)),
					sdk.NewCoin(denom2, sdkmath.NewInt(100)),
				)
				// refund account balance should increase
				refundAccBal := suite.chainA.GetSimApp().BankKeeper.GetAllBalances(suite.chainA.GetContext(), refundAcc)
				suite.Require().Equal(initRefundAccBal.Add(unusedFee...), refundAccBal)

				// module account balance should decrease
				moduleAccBal := suite.chainA.GetSimApp().BankKeeper.GetAllBalances(suite.chainA.GetContext(), moduleAcc)
				suite.Require().Equal(initModuleAccBal.Sub(unusedFee...).Sort(), moduleAccBal)
			},
		},
		{
			"failure: invalid refund address",
			func() {
				fee := types.NewFee(defaultRecvFee, defaultAckFee, defaultTimeoutFee)
				packetFee := types.NewPacketFee(fee, "invalid", []string{})
				packetFees = []types.PacketFee{packetFee}
			},
			func(err error) {
				suite.Require().Error(err)
			},
		},
	}

	for _, tc := range testCases {
		tc := tc

		suite.SetupTest()
		suite.coordinator.Setup(suite.path)

		refundAcc = suite.chainA.SenderAccount.GetAddress()
		moduleAcc = suite.chainA.GetSimApp().AccountKeeper.GetModuleAddress(types.ModuleName)
		packetID = channeltypes.NewPacketID(suite.path.EndpointA.ChannelConfig.PortID, suite.path.EndpointA.ChannelID, 1)
		packetFees = nil

		tc.malleate()

		feesToModule := sdk.NewCoins()
		for _, packetFee := range packetFees {
			feesToModule = feesToModule.Add(keeper.LegacyTotal(packetFee.Fee)...)
		}

		if !feesToModule.IsZero() {
			// escrow the packet fees & store the fees in state
			suite.chainA.GetSimApp().IBCFeeKeeper.SetFeesInEscrow(suite.chainA.GetContext(), packetID, types.NewPacketFees(packetFees))
			err := suite.chainA.GetSimApp().BankKeeper.SendCoinsFromAccountToModule(suite.chainA.GetContext(), refundAcc, types.ModuleName, feesToModule)
			suite.Require().NoError(err)
		}

		initRefundAccBal = suite.chainA.GetSimApp().BankKeeper.GetAllBalances(suite.chainA.GetContext(), refundAcc)
		initModuleAccBal = suite.chainA.GetSimApp().BankKeeper.GetAllBalances(suite.chainA.GetContext(), moduleAcc)

		migrator := keeper.NewMigrator(suite.chainA.GetSimApp().IBCFeeKeeper)
		err := migrator.Migrate1to2(suite.chainA.GetContext())

		tc.assert(err)
	}
}
