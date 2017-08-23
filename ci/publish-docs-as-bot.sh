#!/usr/bin/env bash

# Requires the following command beforehand:
#   $ gem install travis
#   $ travis login --auto
#   $ travis encrypt-file ci/kube-aws-bot-git-ssh-key --repo kubernetes-incubator/kube-aws

openssl aes-256-cbc -K $encrypted_514cf8442810_key -iv $encrypted_514cf8442810_iv -in kube-aws-bot-git-ssh-key.enc -out ci/kube-aws-bot-git-ssh-key -d

echo -e "Host github.com\n\tStrictHostKeyChecking no\nIdentityFile $(pwd)/ci/kube-aws-bot-git-ssh-key\n" >> ~/.ssh/config

make publish-docs
