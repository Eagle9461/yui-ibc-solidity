const Migrations = artifacts.require("Migrations");
const ProvableStore = artifacts.require("ProvableStore");
const IBCClient = artifacts.require("IBCClient");
const IBCConnection = artifacts.require("IBCConnection");
const IBCChannel = artifacts.require("IBCChannel");
const IBCRoutingModule = artifacts.require("IBCRoutingModule");
const SimpleTokenModule = artifacts.require("SimpleTokenModule");
const Bytes = artifacts.require("Bytes");

module.exports = function (deployer) {
  deployer.deploy(Migrations);
  deployer.deploy(Bytes).then(function() {
    return deployer.link(Bytes, [IBCClient, IBCConnection, IBCChannel]);
  });
  deployer.deploy(ProvableStore).then(function() {
    return deployer.deploy(IBCClient, ProvableStore.address).then(function() {
      return deployer.deploy(IBCConnection, ProvableStore.address, IBCClient.address).then(function() {
        return deployer.deploy(IBCChannel, ProvableStore.address, IBCClient.address, IBCConnection.address).then(function() {
          return deployer.deploy(IBCRoutingModule, ProvableStore.address, IBCChannel.address).then(function() {
            return deployer.deploy(SimpleTokenModule, IBCRoutingModule.address);
          });
        });
      });
    });
  });
};
