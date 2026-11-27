// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

// The Mattermost ESLint config (flat, @mattermost/eslint-plugin 2.0.0) from mattermost/mattermost v12.1.0,
// webapp/platform/eslint-plugin/configs/{base,react}.js. 2.0.0 isn't on npm, so it's copied here. Left out:
// the license-header rule and Mattermost's own rules (use-external-link, no-dispatch-getstate,
// no-redundant-admin-config-deps), which target the host webapp's code. Plugin overrides are at the end.

import {fixupConfigRules} from '@eslint/compat';
import js from '@eslint/js';
import stylisticPlugin from '@stylistic/eslint-plugin';
import importPlugin from 'eslint-plugin-import-x';
import jsxA11yPlugin from 'eslint-plugin-jsx-a11y';
import reactPlugin from 'eslint-plugin-react';
import reactHooksPlugin from 'eslint-plugin-react-hooks';
import globals from 'globals';
import tseslint from 'typescript-eslint';

const base = {
    files: ['**/*.js', '**/*.jsx', '**/*.mjs', '**/*.ts', '**/*.tsx'],
    languageOptions: {
        ecmaVersion: 8,
        globals: {
            ...globals.browser,
            ...globals.node,
            ...globals.es6,
        },
        parser: tseslint.parser,
        parserOptions: {
            ecmaFeatures: {
                jsx: true,
                impliedStrict: true,
                modules: true,
            },
        },
        sourceType: 'module',
    },
    plugins: {
        '@stylistic': stylisticPlugin,

        // eslint-plugin-import doesn't run on ESLint 10; import-x is a fork with the same rules.
        import: importPlugin,
    },
    rules: {
        '@stylistic/array-bracket-spacing': [
            2,
            'never',
        ],
        '@stylistic/arrow-parens': [
            2,
            'always',
        ],
        '@stylistic/arrow-spacing': [
            2,
            {
                before: true,
                after: true,
            },
        ],
        '@stylistic/brace-style': [
            2,
            '1tbs',
            {
                allowSingleLine: false,
            },
        ],
        '@stylistic/comma-dangle': [
            2,
            'always-multiline',
        ],
        '@stylistic/comma-spacing': [
            2,
            {
                before: false,
                after: true,
            },
        ],
        '@stylistic/comma-style': [
            2,
            'last',
        ],
        '@stylistic/computed-property-spacing': [
            2,
            'never',
        ],
        '@stylistic/dot-location': [
            2,
            'object',
        ],
        '@stylistic/eol-last': ['error', 'always'],
        '@stylistic/function-call-spacing': [
            2,
            'never',
        ],
        '@stylistic/generator-star-spacing': [
            2,
            {
                before: false,
                after: true,
            },
        ],
        '@stylistic/indent': [
            2,
            4,
            {
                SwitchCase: 0,
            },
        ],
        '@stylistic/jsx-quotes': [
            2,
            'prefer-single',
        ],
        '@stylistic/key-spacing': [
            2,
            {
                beforeColon: false,
                afterColon: true,
                mode: 'strict',
            },
        ],
        '@stylistic/keyword-spacing': [
            2,
            {
                before: true,
                after: true,
                overrides: {},
            },
        ],
        '@stylistic/linebreak-style': 2,
        '@stylistic/lines-around-comment': [
            2,
            {
                beforeBlockComment: true,
                beforeLineComment: true,
                allowBlockStart: true,
                allowBlockEnd: true,
            },
        ],
        '@stylistic/max-statements-per-line': [
            2,
            {
                max: 1,
            },
        ],
        '@stylistic/member-delimiter-style': 2,
        '@stylistic/multiline-ternary': [
            1,
            'never',
        ],
        '@stylistic/new-parens': 2,
        '@stylistic/no-confusing-arrow': 2,
        '@stylistic/no-extra-parens': 0,
        '@stylistic/no-extra-semi': 2,
        '@stylistic/no-floating-decimal': 2,
        '@stylistic/no-mixed-operators': [
            2,
            {
                allowSamePrecedence: false,
            },
        ],
        '@stylistic/no-mixed-spaces-and-tabs': 2,
        '@stylistic/no-multi-spaces': [
            2,
            {
                exceptions: {
                    Property: false,
                },
            },
        ],
        '@stylistic/no-multiple-empty-lines': [
            2,
            {
                max: 1,
            },
        ],
        '@stylistic/no-tabs': 0,
        '@stylistic/no-trailing-spaces': [
            2,
            {
                skipBlankLines: false,
            },
        ],
        '@stylistic/no-whitespace-before-property': 2,
        '@stylistic/object-curly-newline': 0,
        '@stylistic/object-curly-spacing': [
            2,
            'never',
        ],
        '@stylistic/object-property-newline': [
            2,
            {
                allowAllPropertiesOnSameLine: true,
            },
        ],
        '@stylistic/one-var-declaration-per-line': 0,
        '@stylistic/operator-linebreak': [
            2,
            'after',
        ],
        '@stylistic/padded-blocks': [
            2,
            'never',
        ],
        '@stylistic/quote-props': [
            2,
            'as-needed',
        ],
        '@stylistic/quotes': [
            2,
            'single',
            {
                avoidEscape: true,
            },
        ],
        '@stylistic/rest-spread-spacing': [
            2,
            'never',
        ],
        '@stylistic/semi': [
            2,
            'always',
        ],
        '@stylistic/semi-spacing': [
            2,
            {
                before: false,
                after: true,
            },
        ],
        '@stylistic/space-before-blocks': [
            2,
            'always',
        ],
        '@stylistic/space-before-function-paren': [
            2,
            {
                anonymous: 'never',
                named: 'never',
                asyncArrow: 'always',
            },
        ],
        '@stylistic/space-in-parens': [
            2,
            'never',
        ],
        '@stylistic/space-infix-ops': 2,
        '@stylistic/space-unary-ops': [
            2,
            {
                words: true,
                nonwords: false,
            },
        ],
        '@stylistic/template-curly-spacing': [
            2,
            'never',
        ],
        '@stylistic/type-annotation-spacing': 2,
        '@stylistic/wrap-iife': [
            2,
            'outside',
        ],
        '@stylistic/wrap-regex': 2,
        '@typescript-eslint/array-type': [2, {default: 'array-simple'}],
        '@typescript-eslint/consistent-type-imports': ['error', {disallowTypeAnnotations: false}],
        '@typescript-eslint/explicit-function-return-type': 0,
        '@typescript-eslint/explicit-module-boundary-types': 0,
        '@typescript-eslint/naming-convention': [
            2,
            {
                selector: 'function',
                format: ['camelCase', 'PascalCase'],
            },
            {
                selector: 'variable',
                format: ['camelCase', 'PascalCase', 'UPPER_CASE'],
            },
            {
                selector: 'parameter',
                format: ['camelCase', 'PascalCase'],
                leadingUnderscore: 'allow',
            },
            {
                selector: 'typeLike',
                format: ['PascalCase'],
            },
        ],
        '@typescript-eslint/no-dupe-class-members': 2,
        '@typescript-eslint/no-empty-function': 0,
        '@typescript-eslint/no-empty-object-type': [
            2,
            {
                allowInterfaces: 'with-single-extends',
            },
        ],
        '@typescript-eslint/no-explicit-any': 'warn',
        '@typescript-eslint/no-unused-vars': [
            2,
            {
                vars: 'all',
                args: 'after-used',
            },
        ],
        '@typescript-eslint/no-use-before-define': [
            2,
            {
                classes: false,
                functions: false,
                variables: false,
            },
        ],
        '@typescript-eslint/no-var-requires': 0,
        'array-callback-return': 2,
        'arrow-body-style': 0,
        'block-scoped-var': 2,
        'capitalized-comments': 0,
        'class-methods-use-this': 0,
        complexity: [
            0,
            10,
        ],
        'consistent-return': 2,
        'consistent-this': [
            2,
            'self',
        ],
        'constructor-super': 2,
        curly: [
            2,
            'all',
        ],
        'dot-notation': 2,
        eqeqeq: [
            2,
            'smart',
        ],
        'func-name-matching': 0,
        'func-names': 2,
        'func-style': [
            2,
            'declaration',
            {
                allowArrowFunctions: true,
            },
        ],
        'global-require': 2,
        'guard-for-in': 2,
        'id-blacklist': 0,
        'import/no-duplicates': 2,
        'import/no-unresolved': 0, // Handled better by TS
        'import/order': [
            2,
            {
                'newlines-between': 'always',
                groups: [
                    'builtin',
                    'external',
                    'internal',
                    'sibling',
                    'parent',
                    'index',
                ],
                pathGroups: [
                    {
                        pattern: '@mattermost/**',
                        group: 'external',
                        position: 'after',
                    },
                    {
                        pattern: 'mattermost-redux/**',
                        group: 'external',
                        position: 'after',
                    },
                    {
                        pattern: '@(selectors|actions|stores|store|reducers){,/**}',
                        group: 'external',
                        position: 'after',
                    },
                    {
                        pattern: 'components/**',
                        group: 'external',
                        position: 'after',
                    },
                    {
                        pattern: 'types{,/**}',
                        group: 'internal',
                        position: 'after',
                    },
                ],
                alphabetize: {
                    order: 'asc',
                    caseInsensitive: true,
                },
                distinctGroup: true,
                pathGroupsExcludedImportTypes: ['builtin'],
            },
        ],
        'line-comment-position': 0,
        'max-lines': [
            'warn',
            {
                max: 800,
                skipBlankLines: true,
                skipComments: false,
            },
        ],
        'max-nested-callbacks': ['error', 10],
        'new-cap': 2,
        'newline-before-return': 0,
        'newline-per-chained-call': 0,
        'no-alert': 2,
        'no-array-constructor': 2,
        'no-await-in-loop': 2,
        'no-caller': 2,
        'no-case-declarations': 2,
        'no-class-assign': 2,
        'no-compare-neg-zero': 2,
        'no-cond-assign': [
            2,
            'except-parens',
        ],
        'no-console': 2,
        'no-const-assign': 2,
        'no-constant-binary-expression': 2,
        'no-constant-condition': 2,
        'no-debugger': 2,
        'no-div-regex': 2,
        'no-dupe-args': 2,
        'no-dupe-class-members': 0, // Handled by @typescript-eslint/no-dupe-class-members
        'no-dupe-keys': 2,
        'no-duplicate-case': 2,
        'no-duplicate-imports': 0, // Handled by import/no-duplicates
        'no-else-return': 2,
        'no-empty': 2,
        'no-empty-function': 0,
        'no-empty-pattern': 2,
        'no-eval': 2,
        'no-ex-assign': 2,
        'no-extend-native': 2,
        'no-extra-bind': 2,
        'no-extra-label': 2,
        'no-fallthrough': 2,
        'no-func-assign': 2,
        'no-global-assign': 2,
        'no-implicit-coercion': 2,
        'no-implicit-globals': 0,
        'no-implied-eval': 2,
        'no-inner-declarations': 0,
        'no-invalid-regexp': 2,
        'no-irregular-whitespace': 2,
        'no-iterator': 2,
        'no-labels': 2,
        'no-lone-blocks': 2,
        'no-lonely-if': 2,
        'no-loop-func': 2,
        'no-magic-numbers': 0,
        'no-multi-assign': 2,
        'no-multi-str': 0,
        'no-native-reassign': 2,
        'no-negated-condition': 2,
        'no-nested-ternary': 2,
        'no-new': 2,
        'no-new-func': 2,
        'no-new-object': 2,
        'no-new-symbol': 2,
        'no-new-wrappers': 2,
        'no-octal-escape': 2,
        'no-param-reassign': 2,
        'no-process-env': 2,
        'no-process-exit': 2,
        'no-proto': 2,
        'no-prototype-builtins': 2,
        'no-restricted-imports': [
            'error',
            {
                paths: [
                    {
                        name: 'redux',
                        importNames: ['DeepPartial'],
                        message: 'Use DeepPartial from @mattermost/types/utilities instead.',
                    },
                    {
                        name: 'lodash',
                        message: 'Import individual functions from lodash/<function> instead.',
                    },
                ],
            },
        ],
        'no-return-assign': [
            2,
            'always',
        ],
        'no-return-await': 2,
        'no-script-url': 2,
        'no-self-assign': [
            2,
            {
                props: true,
            },
        ],
        'no-self-compare': 2,
        'no-sequences': 2,
        'no-shadow': 0, // This isn't currently enabled, but it probably should be
        'no-shadow-restricted-names': 2,
        'no-template-curly-in-string': 2,
        'no-ternary': 0,
        'no-this-before-super': 2,
        'no-throw-literal': 2,
        'no-undef-init': 2,
        'no-undefined': 0,
        'no-underscore-dangle': 2,
        'no-unexpected-multiline': 2,
        'no-unmodified-loop-condition': 2,
        'no-unneeded-ternary': [
            2,
            {
                defaultAssignment: false,
            },
        ],
        'no-unreachable': 2,
        'no-unsafe-finally': 2,
        'no-unsafe-negation': 2,
        'no-unused-expressions': 2,
        'no-unused-vars': 0, // Handled by @typescript-eslint/no-unused-vars
        'no-use-before-define': 0, // Handled by @typescript-eslint/no-use-before-define
        'no-useless-computed-key': 2,
        'no-useless-concat': 2,
        'no-useless-constructor': 2,
        'no-useless-escape': 2,
        'no-useless-rename': 2,
        'no-useless-return': 2,
        'no-var': 0,
        'no-void': 2,
        'no-warning-comments': 1,
        'no-with': 2,
        'object-shorthand': [
            2,
            'always',
        ],
        'one-var': [
            2,
            'never',
        ],
        'operator-assignment': [
            2,
            'always',
        ],
        'prefer-arrow-callback': 2,
        'prefer-const': 2,
        'prefer-destructuring': 0,
        'prefer-numeric-literals': 2,
        'prefer-promise-reject-errors': 2,
        'prefer-rest-params': 2,
        'prefer-spread': 2,
        'prefer-template': 0,
        radix: 2,
        'require-yield': 2,
        'sort-imports': 0,
        'sort-keys': 0,
        'symbol-description': 2,
        'valid-typeof': [
            2,
            {
                requireStringLiterals: false,
            },
        ],
        'vars-on-top': 0,
        yoda: [
            2,
            'never',
            {
                exceptRange: false,
                onlyEquality: false,
            },
        ],
    },
};

const testOverrides = {
    files: ['**/*.test.js', '**/*.test.jsx', '**/*.test.ts', '**/*.test.tsx', 'src/tests/**'],
    languageOptions: {
        globals: {
            after: true,
            afterAll: true,
            afterEach: true,
            before: true,
            beforeAll: true,
            beforeEach: true,
            describe: true,
            expect: true,
            it: true,
            jest: true,
            test: true,
        },
    },
    rules: {
        'func-names': 0,
        'global-require': 0,
        'no-console': 0,
        'no-import-assign': 0,
        'max-lines': 0,
        'max-nested-callbacks': 0,
        'new-cap': 0,
        'prefer-arrow-callback': 0,
        '@typescript-eslint/no-require-imports': 0,
    },
};

const react = {
    files: ['**/*.jsx', '**/*.tsx'],
    rules: {
        '@stylistic/jsx-closing-bracket-location': [
            2,
            {
                location: 'tag-aligned',
            },
        ],
        '@stylistic/jsx-curly-spacing': [
            2,
            'never',
        ],
        '@stylistic/jsx-equals-spacing': [
            2,
            'never',
        ],
        '@stylistic/jsx-first-prop-new-line': [
            2,
            'multiline',
        ],
        '@stylistic/jsx-indent-props': [
            2,
            4,
        ],
        '@stylistic/jsx-max-props-per-line': [
            2,
            {
                maximum: 1,
            },
        ],
        '@stylistic/jsx-pascal-case': 2,
        '@stylistic/jsx-tag-spacing': [
            2,
            {
                closingSlash: 'never',
                beforeSelfClosing: 'never',
                afterOpening: 'never',
            },
        ],
        '@stylistic/jsx-wrap-multilines': 2,
        'jsx-a11y/alt-text': 'warn',
        'jsx-a11y/anchor-has-content': 'error',
        'jsx-a11y/anchor-is-valid': 'warn',
        'jsx-a11y/aria-activedescendant-has-tabindex': 'error',
        'jsx-a11y/aria-props': 'error',
        'jsx-a11y/aria-proptypes': 'error',
        'jsx-a11y/aria-role': 'error',
        'jsx-a11y/aria-unsupported-elements': 'error',
        'jsx-a11y/autocomplete-valid': 'error',
        'jsx-a11y/click-events-have-key-events': 'warn',
        'jsx-a11y/heading-has-content': 'error',
        'jsx-a11y/html-has-lang': 'error',
        'jsx-a11y/iframe-has-title': 'error',
        'jsx-a11y/img-redundant-alt': 'warn',
        'jsx-a11y/interactive-supports-focus': 'warn',
        'jsx-a11y/label-has-associated-control': 'error',
        'jsx-a11y/media-has-caption': 'warn',
        'jsx-a11y/mouse-events-have-key-events': 'off',
        'jsx-a11y/no-access-key': 'error',
        'jsx-a11y/no-aria-hidden-on-focusable': 'error',
        'jsx-a11y/no-autofocus': 'warn',
        'jsx-a11y/no-distracting-elements': 'error',
        'jsx-a11y/no-interactive-element-to-noninteractive-role': 'error',
        'jsx-a11y/no-noninteractive-element-interactions': 'warn',
        'jsx-a11y/no-noninteractive-element-to-interactive-role': 'warn',
        'jsx-a11y/no-noninteractive-tabindex': 'warn',
        'jsx-a11y/no-redundant-roles': 'warn',
        'jsx-a11y/no-static-element-interactions': 'warn',
        'jsx-a11y/role-has-required-aria-props': 'warn',
        'jsx-a11y/role-supports-aria-props': 'warn',
        'jsx-a11y/scope': 'error',
        'jsx-a11y/tabindex-no-positive': 'warn',
        'react/display-name': [
            0,
            {
                ignoreTranspilerName: false,
            },
        ],
        'react/forbid-component-props': 0,
        'react/forbid-elements': [
            2,
            {
                forbid: [
                    'embed',
                ],
            },
        ],
        'react/jsx-boolean-value': [
            2,
            'always',
        ],
        'react/jsx-filename-extension': [
            2,
            {
                extensions: [
                    '.jsx',
                    '.tsx',
                ],
            },
        ],
        'react/jsx-handler-names': 0,
        'react/jsx-key': 2,
        'react/jsx-no-bind': 0,
        'react/jsx-no-comment-textnodes': 2,
        'react/jsx-no-duplicate-props': [
            2,
            {
                ignoreCase: false,
            },
        ],
        'react/jsx-no-literals': 2,
        'react/jsx-no-target-blank': 2,
        'react/jsx-no-undef': 2,
        'react/jsx-uses-react': 2,
        'react/jsx-uses-vars': 2,
        'react/no-array-index-key': 1,
        'react/no-children-prop': 2,
        'react/no-danger': 0,
        'react/no-danger-with-children': 2,
        'react/no-deprecated': 1,
        'react/no-did-mount-set-state': 2,
        'react/no-did-update-set-state': 2,
        'react/no-direct-mutation-state': 2,
        'react/no-find-dom-node': 1,
        'react/no-is-mounted': 2,
        'react/no-multi-comp': [
            2,
            {
                ignoreStateless: true,
            },
        ],
        'react/no-render-return-value': 2,
        'react/no-set-state': 0,
        'react/no-string-refs': 2,
        'react/no-unescaped-entities': 2,
        'react/no-unknown-property': [
            2,
            {
                ignore: ['mask-type'],
            },
        ],
        'react/no-unused-prop-types': [
            1,
            {
                skipShapeProps: true,
            },
        ],
        'react/prefer-es6-class': 2,
        'react/prefer-stateless-function': 0,
        'react/prop-types': 2,
        'react/require-default-props': 0,
        'react/require-optimization': 1,
        'react/require-render-return': 2,
        'react/self-closing-comp': 2,
        'react/sort-comp': 0,
        'react/style-prop-object': 2,
    },
};

export default [
    {
        ignores: ['dist/', 'src/manifest.ts', 'webpack.config.js'],
    },
    js.configs.recommended,
    ...tseslint.configs.recommended,
    stylisticPlugin.configs['disable-legacy'],
    base,
    testOverrides,
    jsxA11yPlugin.flatConfigs.recommended,

    // eslint-plugin-react still calls context methods that ESLint 10 removed.
    ...fixupConfigRules(reactPlugin.configs.flat.recommended),
    reactHooksPlugin.configs.flat.recommended,
    react,
    {

        // Disable new rules that are primarily needed to enable React Compiler because we're not using that yet
        rules: {
            'react-hooks/immutability': 0,
            'react-hooks/preserve-manual-memoization': 0,
            'react-hooks/purity': 0,
            'react-hooks/refs': 0,
            'react-hooks/set-state-in-effect': 0,
            'react-hooks/static-components': 0,
        },
    },
    {
        settings: {
            react: {
                pragma: 'React',
                version: 'detect',
            },
        },
    },

    // Plugin overrides.
    {
        rules: {
            'react/jsx-uses-react': 'off',
            'react/react-in-jsx-scope': 'off',
            'react/no-danger': 'error',
            'react/jsx-no-script-url': 'error',
        },
    },
    {
        files: ['babel.config.js'],
        rules: {
            '@typescript-eslint/no-require-imports': 0,
            'global-require': 0,
        },
    },
];
