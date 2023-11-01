// require('dotenv').config();

// console.log(process.env);

const config = require('config');

const dbConfig = config.get('db');

console.log('🚀 ~ file: index.js ~ line 7 ~ dbConfig', dbConfig);
