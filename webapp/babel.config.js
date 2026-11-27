// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

module.exports = {

    // Matches the browsers supported by the Mattermost web app.
    targets: {
        chrome: 110,
        firefox: 102,
        edge: 110,
        safari: '16.4',
    },
    presets: [
        ['@babel/preset-env', {
            shippedProposals: true,
        }],

        // The automatic runtime imports react/jsx-runtime, which webpack maps to the host's ReactJSXRuntime.
        ['@babel/preset-react', {
            runtime: 'automatic',
        }],
        '@babel/preset-typescript',
    ],
};
