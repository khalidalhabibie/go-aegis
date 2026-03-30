// SPDX-License-Identifier: MIT
pragma solidity 0.8.33;

import {Test} from "forge-std/Test.sol";

import {AegisEscrow} from "../src/AegisEscrow.sol";

contract AegisEscrowTest is Test {
    AegisEscrow internal escrow;

    address internal admin = makeAddr("admin");
    address internal payer = makeAddr("payer");
    address internal recipient = makeAddr("recipient");

    uint256 internal constant ESCROW_ID = 1;
    uint256 internal constant DEPOSIT_AMOUNT = 1 ether;

    event Deposited(uint256 indexed escrowId, address indexed payer, address indexed recipient, uint256 amount);
    event Released(uint256 indexed escrowId, address indexed recipient, uint256 amount);
    event Refunded(uint256 indexed escrowId, address indexed payer, uint256 amount);

    function setUp() external {
        escrow = new AegisEscrow(admin);
        vm.deal(payer, 10 ether);
    }

    function testDepositStoresEscrowAndEmitsEvent() external {
        vm.prank(payer);
        vm.expectEmit(true, true, true, true);
        emit Deposited(ESCROW_ID, payer, recipient, DEPOSIT_AMOUNT);
        escrow.deposit{value: DEPOSIT_AMOUNT}(ESCROW_ID, recipient);

        AegisEscrow.Escrow memory stored = escrow.getEscrow(ESCROW_ID);

        assertEq(stored.payer, payer);
        assertEq(stored.recipient, recipient);
        assertEq(stored.amount, DEPOSIT_AMOUNT);
        assertEq(uint256(stored.status), uint256(AegisEscrow.Status.Funded));
    }

    function testReleaseTransfersFundsAndUpdatesStatus() external {
        vm.prank(payer);
        escrow.deposit{value: DEPOSIT_AMOUNT}(ESCROW_ID, recipient);

        uint256 recipientBalanceBefore = recipient.balance;

        vm.prank(admin);
        vm.expectEmit(true, true, true, true);
        emit Released(ESCROW_ID, recipient, DEPOSIT_AMOUNT);
        escrow.release(ESCROW_ID);

        AegisEscrow.Escrow memory stored = escrow.getEscrow(ESCROW_ID);

        assertEq(recipient.balance, recipientBalanceBefore + DEPOSIT_AMOUNT);
        assertEq(stored.amount, 0);
        assertEq(uint256(stored.status), uint256(AegisEscrow.Status.Released));
    }

    function testRefundTransfersFundsBackToPayer() external {
        vm.prank(payer);
        escrow.deposit{value: DEPOSIT_AMOUNT}(ESCROW_ID, recipient);

        uint256 payerBalanceBefore = payer.balance;

        vm.prank(admin);
        vm.expectEmit(true, true, true, true);
        emit Refunded(ESCROW_ID, payer, DEPOSIT_AMOUNT);
        escrow.refund(ESCROW_ID);

        AegisEscrow.Escrow memory stored = escrow.getEscrow(ESCROW_ID);

        assertEq(payer.balance, payerBalanceBefore + DEPOSIT_AMOUNT);
        assertEq(stored.amount, 0);
        assertEq(uint256(stored.status), uint256(AegisEscrow.Status.Refunded));
    }

    function testOnlyAdminCanRelease() external {
        vm.prank(payer);
        escrow.deposit{value: DEPOSIT_AMOUNT}(ESCROW_ID, recipient);

        vm.prank(payer);
        vm.expectRevert(abi.encodeWithSignature("OwnableUnauthorizedAccount(address)", payer));
        escrow.release(ESCROW_ID);
    }

    function testDepositRevertsOnZeroValue() external {
        vm.prank(payer);
        vm.expectRevert(AegisEscrow.ZeroDeposit.selector);
        escrow.deposit(ESCROW_ID, recipient);
    }

    function testDepositRevertsOnDuplicateEscrowId() external {
        vm.startPrank(payer);
        escrow.deposit{value: DEPOSIT_AMOUNT}(ESCROW_ID, recipient);
        vm.expectRevert(abi.encodeWithSelector(AegisEscrow.EscrowAlreadyExists.selector, ESCROW_ID));
        escrow.deposit{value: DEPOSIT_AMOUNT}(ESCROW_ID, recipient);
        vm.stopPrank();
    }
}
