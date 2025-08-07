// SPDX-License-Identifier: GPL-3.0
pragma solidity ^0.8.28;

import {ERC20 as OZ_ERC20} from "@openzeppelin/token/ERC20/ERC20.sol" ;

contract ERC20 is OZ_ERC20 {
    constructor() OZ_ERC20("token", "gasperToken") {
        _mint(msg.sender, 10000 * (10 ** uint256(decimals())));
    }

    function mint(uint256 amount) external {
        _mint(msg.sender, amount);
    }
}
