#!/usr/bin/env node

'use strict'

const semver = require('semver')

// Check if a given version is a higher major version
// than the specified version in $2.

/**
 * @returns {Number} exit code
 */
const main = () => {
  let lastVersion = process.argv[2]
  let newVersion = process.argv[3]
  if (!newVersion) {
    // default to v1 for last version if we only got
    // a new version
    newVersion = lastVersion
    lastVersion = 'v1.0.0'
  }

  const lastMajor = semver.major(lastVersion)
  const newMajor = semver.major(newVersion)


  if (newMajor != lastMajor) {
    console.error('Detected major version upgrade. Failing ...')
    return 2
  }

  return 0
}

try {
  process.exitCode = main()
} catch (err) {
  console.error('failed to check version:', err.message || err)
  process.exitCode = 1
}
