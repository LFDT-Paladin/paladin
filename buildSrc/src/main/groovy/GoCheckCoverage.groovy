import org.gradle.api.DefaultTask
import org.gradle.api.GradleException
import org.gradle.api.tasks.Input
import org.gradle.api.tasks.InputFile
import org.gradle.api.tasks.TaskAction

import java.math.RoundingMode

class GoCheckCoverage extends DefaultTask {

    // Cap on the uncovered blocks named in a failure, so the message stays readable on a large drop.
    private static final int MAX_REPORTED_BLOCKS = 25

    @InputFile
    File coverageFile

    @Input
    BigDecimal target

    @Input
    BigDecimal maxGap

    void coverageFile(Object f) {
        coverageFile = project.file(f)
    }

    @TaskAction
    void exec() {
        // Statements are counted straight from the coverage profile rather than read off
        // `go tool cover -func`, whose total is rounded to one decimal place. That rounding is enough for a
        // handful of uncovered statements to report as 100.0%, so a 100% target could be met without the
        // coverage actually being complete.
        // Profile lines are "<file>:<startLine>.<col>,<endLine>.<col> <numStatements> <hitCount>". A profile
        // built with -coverpkg repeats every block once per test binary, so the hit counts are accumulated
        // per block and the statements counted once, exactly as `go tool cover` merges them.
        Map<String, Long> statementsByBlock = new LinkedHashMap<>()
        Map<String, Long> hitsByBlock = new LinkedHashMap<>()
        coverageFile.eachLine { String line ->
            String trimmed = line.trim()
            if (trimmed.isEmpty() || trimmed.startsWith('mode:')) {
                return
            }
            String[] fields = trimmed.split('\\s+')
            if (fields.length < 3) {
                throw new GradleException("ERROR: malformed line in coverage profile ${coverageFile}: ${line}")
            }
            long statements = Long.parseLong(fields[fields.length - 2])
            if (statements == 0) {
                return // a block with no statements in it counts towards neither total
            }
            String position = fields[0]
            long hits = Long.parseLong(fields[fields.length - 1])
            statementsByBlock.putIfAbsent(position, statements)
            hitsByBlock.put(position, (hitsByBlock.get(position) ?: 0L) + hits)
        }

        long totalStatements = 0
        long coveredStatements = 0
        List<String> uncoveredBlocks = []
        statementsByBlock.each { String position, Long statements ->
            totalStatements += statements
            if (hitsByBlock.get(position) > 0) {
                coveredStatements += statements
            } else {
                uncoveredBlocks << position
            }
        }

        if (totalStatements == 0) {
            throw new GradleException("ERROR: no statements found in coverage profile ${coverageFile}")
        }

        // Truncated, never rounded, so the figure only ever reads lower than reality and full coverage is the
        // one and only way to reach 100%.
        BigDecimal coverage = BigDecimal.valueOf(coveredStatements)
            .multiply(100G)
            .divide(BigDecimal.valueOf(totalStatements), 6, RoundingMode.DOWN)
        String percentage = coverage.stripTrailingZeros().toPlainString()

        println "Coverage is ${percentage}% (${coveredStatements}/${totalStatements} statements)"
        if (coverage < target) {
            throw new GradleException(
                "ERROR: Coverage is below ${target}% (current coverage: ${percentage}%, " +
                "${totalStatements - coveredStatements} of ${totalStatements} statements uncovered)" +
                uncoveredReport(uncoveredBlocks)
            )
        } else if (coverage - target > maxGap) {
            throw new GradleException(
                "ERROR: The target coverage ${target}% is below the current coverage: ${percentage}% by " +
                "more than ${maxGap}%; please update the target value in build.gradle."
            )
        } else {
            println "Coverage meets the target of ${target}%, current coverage: ${percentage}%"
        }
    }

    private static String uncoveredReport(List<String> uncoveredBlocks) {
        if (uncoveredBlocks.isEmpty()) {
            return ''
        }
        List<String> sorted = uncoveredBlocks.toSorted()
        StringBuilder report = new StringBuilder('\nUncovered blocks:')
        sorted.take(MAX_REPORTED_BLOCKS).each { report.append('\n  ').append(it) }
        if (sorted.size() > MAX_REPORTED_BLOCKS) {
            report.append("\n  ...and ${sorted.size() - MAX_REPORTED_BLOCKS} more")
        }
        return report.toString()
    }

}
