#define _POSIX_C_SOURCE 200809L
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static int log_command(int argc, char **argv) {
    const char *path = getenv("MI_LSP_TEST_LOG");
    FILE *log;
    int i;
    if (path == NULL || (log = fopen(path, "a")) == NULL) {
        perror("mi-lsp fixture log");
        return 1;
    }
    for (i = 1; i < argc; ++i) {
        if (i != 1 && fputc(' ', log) == EOF) {
            fclose(log);
            return 1;
        }
        if (fputs(argv[i], log) == EOF) {
            fclose(log);
            return 1;
        }
    }
    if (fputc('\n', log) == EOF || fclose(log) != 0) {
        perror("mi-lsp fixture log write");
        return 1;
    }
    return 0;
}

int main(int argc, char **argv) {
    if (argc == 2 && strcmp(argv[1], "hold") == 0) {
        for (;;) {
            if (pause() == -1 && errno != EINTR) {
                perror("mi-lsp fixture pause");
                return 1;
            }
        }
    }
    if (argc == 5 && strcmp(argv[1], "daemon") == 0 &&
        strcmp(argv[2], "stop") == 0 && strcmp(argv[3], "--format") == 0 &&
        strcmp(argv[4], "compact") == 0) {
        return log_command(argc, argv);
    }
    fprintf(stderr, "unexpected fixture CLI arguments\n");
    return 64;
}
