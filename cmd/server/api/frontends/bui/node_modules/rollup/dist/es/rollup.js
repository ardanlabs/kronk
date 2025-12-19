/*
  @license
	Rollup.js v4.53.5
	Tue, 16 Dec 2025 06:14:08 GMT - commit 31bb66ee9eea35e5ae348e4074bbad55d390112b

	https://github.com/rollup/rollup

	Released under the MIT License.
*/
export { version as VERSION, defineConfig, rollup, watch } from './shared/node-entry.js';
import './shared/parseAst.js';
import '../native.js';
import 'node:path';
import 'path';
import 'node:process';
import 'node:perf_hooks';
import 'node:fs/promises';
