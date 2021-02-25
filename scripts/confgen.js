const IBCStore = artifacts.require("IBCStore");
const IBFT2Client = artifacts.require("IBFT2Client");
const IBCClient = artifacts.require("IBCClient");
const IBCConnection = artifacts.require("IBCConnection");
const IBCChannel = artifacts.require("IBCChannel");
const IBCRoutingModule = artifacts.require("IBCRoutingModule");
const IBCIdentifier = artifacts.require("IBCIdentifier");
const SimpleTokenModule = artifacts.require("SimpleTokenModule");

var fs = require("fs");
var ejs = require("ejs");

if (!process.env.CONF_TPL) {
  console.log("You must set environment variable 'CONF_TPL'");
  process.exit(1);
}

const makePairs = function(arr) {
  var pairs = [];
  for (var i=0 ; i<arr.length ; i+=2) {
      if (arr[i+1] !== undefined) {
          pairs.push ([arr[i], arr[i+1]]);
      } else {
          console.error("invalid pair found");
          process.exit(1);
      }
  }
  return pairs;
};

const targets = makePairs(process.env.CONF_TPL.split(":"));

module.exports = function(callback) {
  targets.forEach(function(item) {
    ejs.renderFile(item[1], {
      IBCStoreAddress: IBCStore.address,
      IBCClientAddress: IBCClient.address,
      IBCConnectionAddress: IBCConnection.address,
      IBCChannelAddress: IBCChannel.address,
      IBCRoutingModuleAddress: IBCRoutingModule.address,
      IBCIdentifierAddress: IBCIdentifier.address,
      IBFT2ClientAddress: IBFT2Client.address,
      SimpleTokenModuleAddress: SimpleTokenModule.address
    }, null, function(err, str){
        if (err) {
          throw err;
        }
        fs.writeFileSync(item[0], str);
        console.log('generated file', item[0]);
      });
  });
  callback();
};
