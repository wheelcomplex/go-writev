#include <stdlib.h>
#include <stdio.h>
#include <sys/socket.h>
#include <arpa/inet.h>
#include <unistd.h>
#include <time.h>
#include <signal.h>

static volatile sig_atomic_t exit_signal = 0;

void handle_sighup(int signum) 
{
    /* in case we registered this handler for multiple signals */ 
    if (signum == SIGHUP || signum == SIGTERM || signum == SIGINT) {
        exit_signal = 1;
    }
}

int main(int argc, char** argv)
{
    printf("the writev example for https://github.com/golang/go/issues/13451\n");

    if (argc < 2) {
        printf("Usage: %s <port>\n"
            "   port: the tcp port to connect to.\n"
            "For example:\n"
            "   %s 1985\n", argv[0], argv[0]);
        exit(-1);
    }

    int port = ::atoi(argv[1]);

    int fd = ::socket(AF_INET, SOCK_STREAM, 0);
    if (fd == -1) {
        printf("create socket failed.\n");
        exit(-1);
    }

    sockaddr_in addr;
    addr.sin_family = AF_INET;
    addr.sin_port = htons(port);
    addr.sin_addr.s_addr = inet_addr("127.0.0.1");

    if(::connect(fd, (const struct sockaddr*)&addr, sizeof(sockaddr_in)) < 0) {
        printf("connect to server at tcp://%d failed.\n", port);
        exit(-1);
    }
    printf("server at tcp://%d connected, press <ctl+c> to exit ...\n", port);

    /* you may also prefer sigaction() instead of signal() */
    signal(SIGHUP, handle_sighup);
    signal(SIGTERM, handle_sighup);
    signal(SIGINT, handle_sighup);

    // read util EOF.
    // big buf size to reduce CPU usage of client
    // 64MB buffer
    const int buffersize = 65535;
    char *buf = (char*)malloc( buffersize*sizeof(char));
    int rd;
    int emptycount;
    int64_t readbytes;
    time_t start, end;
    time (&start);
    while (exit_signal == 0) {
        rd = ::read(fd, buf, buffersize);
        if (rd == -1) {
            printf("server closed.\n");
            break;
        }
        if (rd == 0) {
            emptycount++;
            if (emptycount > 2048) {
                printf("Too many empty read on socket, server closed?\n");
                break;
            }
            continue;
        }
        readbytes += rd;
    }
    if (exit_signal > 0) {
        printf("\nclient exit by signal.\n");
    }
    time (&end);
    double dif = difftime (end, start);
    if (dif > 0) {
        printf ("Elasped time is %.2lf seconds, bandwidth %.2lf MB/s.\n", dif, readbytes / 1024 / 1024 / dif );
    }

    ::close(fd);
    free(buf);

    return 0;
}
