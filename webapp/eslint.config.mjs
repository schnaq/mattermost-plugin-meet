// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {fixupConfigRules} from '@eslint/compat';
import js from '@eslint/js';
import stylistic from '@stylistic/eslint-plugin';
import importX from 'eslint-plugin-import-x';
import jsxA11y from 'eslint-plugin-jsx-a11y';
import react from 'eslint-plugin-react';
import reactHooks from 'eslint-plugin-react-hooks';
import globals from 'globals';
import tseslint from 'typescript-eslint';

export default [
    {ignores: ['dist/', 'src/manifest.ts', 'webpack.config.js']},
    js.configs.recommended,
    ...tseslint.configs.recommended,

    // eslint-plugin-react still calls context methods that ESLint 10 removed.
    ...fixupConfigRules(react.configs.flat.recommended),
    reactHooks.configs.flat.recommended,
    jsxA11y.flatConfigs.recommended,
    {
        languageOptions: {globals: {...globals.browser, ...globals.node}},
        plugins: {'@stylistic': stylistic, import: importX},
        settings: {react: {version: 'detect'}},
        rules: {
            // Babel drops only `import type`, so type-only imports must say so.
            '@typescript-eslint/consistent-type-imports': ['error', {disallowTypeAnnotations: false}],
            '@typescript-eslint/no-explicit-any': 'off',
            'react/no-danger': 'error',
            'react/jsx-no-script-url': 'error',
            'react/react-in-jsx-scope': 'off',
            'jsx-a11y/no-noninteractive-element-interactions': 'warn',

            // Rules for the React Compiler, which the plugin doesn't use.
            'react-hooks/immutability': 'off',
            'react-hooks/refs': 'off',
            'react-hooks/set-state-in-effect': 'off',

            'import/no-duplicates': 'error',
            'import/order': ['error', {
                groups: ['builtin', 'external', 'internal', 'sibling', 'parent', 'index'],
                pathGroups: [
                    {pattern: '@mattermost/**', group: 'external', position: 'after'},
                    {pattern: 'mattermost-redux/**', group: 'external', position: 'after'},
                ],
                pathGroupsExcludedImportTypes: ['builtin'],
                distinctGroup: true,
                'newlines-between': 'always',
                alphabetize: {order: 'asc', caseInsensitive: true},
            }],

            '@stylistic/indent': ['error', 4, {SwitchCase: 0}],
            '@stylistic/quotes': ['error', 'single', {avoidEscape: true}],
            '@stylistic/jsx-quotes': ['error', 'prefer-single'],
            '@stylistic/semi': ['error', 'always'],
            '@stylistic/comma-dangle': ['error', 'always-multiline'],
            '@stylistic/object-curly-spacing': ['error', 'never'],
            '@stylistic/arrow-parens': ['error', 'always'],
            '@stylistic/brace-style': ['error', '1tbs'],
            '@stylistic/eol-last': 'error',
            '@stylistic/no-trailing-spaces': 'error',
            '@stylistic/no-multiple-empty-lines': ['error', {max: 1}],
            '@stylistic/member-delimiter-style': 'error',
            '@stylistic/jsx-indent-props': ['error', 4],
            '@stylistic/jsx-max-props-per-line': ['error', {maximum: 1}],
            '@stylistic/jsx-first-prop-new-line': ['error', 'multiline'],
            '@stylistic/jsx-closing-bracket-location': ['error', 'tag-aligned'],
        },
    },
    {
        files: ['babel.config.js'],
        rules: {'@typescript-eslint/no-require-imports': 'off'},
    },
];
