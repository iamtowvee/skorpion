#include <pthread.h>
#include <stdio.h>
#include <stdatomic.h>

typedef struct { _Atomic int done; pthread_mutex_t mtx; pthread_cond_t cv; } P;

void* worker(void* arg) {
    P* p = arg;
    pthread_mutex_lock(&p->mtx);
    atomic_store(&p->done, 1);
    pthread_cond_broadcast(&p->cv);
    pthread_mutex_unlock(&p->mtx);
    return NULL;
}

int main() {
    P p;
    atomic_store(&p.done, 0);
    pthread_mutex_init(&p.mtx, NULL);
    pthread_cond_init(&p.cv, NULL);
    pthread_t t;
    pthread_create(&t, NULL, worker, &p);
    pthread_detach(t);
    pthread_mutex_lock(&p.mtx);
    while (!atomic_load(&p.done)) {
        pthread_cond_wait(&p.cv, &p.mtx);
    }
    pthread_mutex_unlock(&p.mtx);
    printf("done\n");
    return 0;
}