// SPDX-License-Identifier: MIT
pragma solidity 0.8.33;

import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

contract AegisEscrow is Ownable, ReentrancyGuard {
    enum Status {
        None,
        Funded,
        Released,
        Refunded
    }

    struct Escrow {
        address payer;
        address recipient;
        uint256 amount;
        Status status;
    }

    error DirectTransferNotAllowed();
    error InvalidAdmin();
    error InvalidRecipient();
    error EscrowAlreadyExists(uint256 escrowId);
    error EscrowNotFunded(uint256 escrowId);
    error ZeroDeposit();
    error NativeTransferFailed();

    mapping(uint256 escrowId => Escrow) private _escrows;

    event Deposited(
        uint256 indexed escrowId,
        address indexed payer,
        address indexed recipient,
        uint256 amount
    );
    event Released(
        uint256 indexed escrowId,
        address indexed recipient,
        uint256 amount
    );
    event Refunded(
        uint256 indexed escrowId,
        address indexed payer,
        uint256 amount
    );

    constructor(address admin) Ownable(admin) {
        if (admin == address(0)) {
            revert InvalidAdmin();
        }
    }

    receive() external payable {
        revert DirectTransferNotAllowed();
    }

    function deposit(uint256 escrowId, address recipient) external payable nonReentrant {
        if (msg.value == 0) {
            revert ZeroDeposit();
        }
        if (recipient == address(0)) {
            revert InvalidRecipient();
        }
        if (_escrows[escrowId].status != Status.None) {
            revert EscrowAlreadyExists(escrowId);
        }

        _escrows[escrowId] = Escrow({
            payer: msg.sender,
            recipient: recipient,
            amount: msg.value,
            status: Status.Funded
        });

        emit Deposited(escrowId, msg.sender, recipient, msg.value);
    }

    function release(uint256 escrowId) external onlyOwner nonReentrant {
        Escrow storage escrow = _escrows[escrowId];
        if (escrow.status != Status.Funded) {
            revert EscrowNotFunded(escrowId);
        }

        uint256 amount = escrow.amount;
        address recipient = escrow.recipient;

        escrow.amount = 0;
        escrow.status = Status.Released;

        (bool sent,) = recipient.call{value: amount}("");
        if (!sent) {
            revert NativeTransferFailed();
        }

        emit Released(escrowId, recipient, amount);
    }

    function refund(uint256 escrowId) external onlyOwner nonReentrant {
        Escrow storage escrow = _escrows[escrowId];
        if (escrow.status != Status.Funded) {
            revert EscrowNotFunded(escrowId);
        }

        uint256 amount = escrow.amount;
        address payer = escrow.payer;

        escrow.amount = 0;
        escrow.status = Status.Refunded;

        (bool sent,) = payer.call{value: amount}("");
        if (!sent) {
            revert NativeTransferFailed();
        }

        emit Refunded(escrowId, payer, amount);
    }

    function getEscrow(uint256 escrowId) external view returns (Escrow memory) {
        return _escrows[escrowId];
    }

    function statusOf(uint256 escrowId) external view returns (Status) {
        return _escrows[escrowId].status;
    }
}

