const path = require('path');

module.exports = {
  entry: './src/js/index.js',
  mode: 'production',
  module: {
    rules: [
      {
        test: /\.css$/i,
        use: ["style-loader", "css-loader"],
      },
    ],
  },
  output: {
      filename: 'main.bundle.js',
      path: path.resolve(__dirname, 'assets'),
  },
};
